// Run against Vite. Exercises background query refreshes while task creation is open.
const { chromium } = require(process.env.KAMACU_PLAYWRIGHT_PACKAGE || 'playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

(async () => {
  const filename = `__task-model-state-${process.pid}.tsx`;
  const harness = path.join(__dirname, '../web/src', filename);
  // A temporary Vite entry uses the real component and QueryClient without
  // adding test hooks or exports to the production app.
  fs.writeFileSync(harness, `
import React from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { MemoryRouter } from 'react-router';
import { NewTaskDialog } from './components/board/NewTaskDialog';
import './index.css';
const agents = [
  { id: 1, name: 'Claude', engine: 'claude', models: ['sonnet'], model_argument: '--model {{model}}' },
  { id: 2, name: 'Custom', engine: 'custom', models: ['large'], model_argument: '--model {{model}}' },
];
const client = new QueryClient({defaultOptions: {queries: {staleTime: Infinity, retry: false}}});
client.setQueryData(['agents'], agents);
client.setQueryData(['projects'], [{id: 1, agent_id: 1}]);
Object.assign(window, { modelReview: {
  changeDefault: (agentId: number) => client.setQueryData(['projects'], [{id: 1, agent_id: agentId}]),
  removeModels: () => client.setQueryData(['agents'], agents.map(agent => agent.id === 2 ? {...agent, models: []} : agent)),
}});
createRoot(document.getElementById('root')!).render(
  <QueryClientProvider client={client}><MemoryRouter><NewTaskDialog projectId={1} open onOpenChange={() => {}} /></MemoryRouter></QueryClientProvider>
);
`, { flag: 'wx' });
  let browser;
  try {
    browser = await chromium.launch({ headless: true, executablePath: process.env.KAMACU_BROWSER_EXECUTABLE, args: ['--no-sandbox'] });
    const page = await browser.newPage();
    page.setDefaultTimeout(10000);
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    let created;
    await page.route('**/__task-model-state-check', route => route.fulfill({ contentType: 'text/html', body: `
<html><head><script type="module">
import { injectIntoGlobalHook } from '/@react-refresh';
injectIntoGlobalHook(window); window.$RefreshReg$ = () => {}; window.$RefreshSig$ = () => type => type;
</script></head><body><div id="root"></div><script type="module" src="/src/${filename}"></script></body></html>` }));
    await page.route('**/api/projects/1/tasks', route => {
      created = route.request().postDataJSON();
      return route.fulfill({ json: { id: 1, project_id: 1, ...created } });
    });
    await page.goto(process.argv[2] + '/__task-model-state-check');
    const model = page.getByLabel('Model', { exact: true });
    await model.selectOption('sonnet');
    await page.evaluate(() => window.modelReview.changeDefault(2));
    await model.locator('option[value="large"]').waitFor({ state: 'attached' });
    assert.equal(await model.inputValue(), '', 'Refreshing the project agent must clear an incompatible model');
    await model.selectOption('large');
    await page.evaluate(() => window.modelReview.removeModels());
    await page.waitForFunction(() => document.getElementById('task-model').disabled);
    assert.equal(await model.inputValue(), '', 'Removing the selected curated model must restore Agent default');
    await page.getByPlaceholder('Task title', { exact: true }).fill('Refreshed selection');
    await Promise.all([page.waitForResponse(response => response.url().endsWith('/api/projects/1/tasks')), page.getByRole('button', { name: 'Create task', exact: true }).click()]);
    assert.deepEqual(created, { title: 'Refreshed selection', description: '', agent_id: 2, model: '' });
    assert.deepEqual(errors, []);
    console.log('Verified project-agent refresh and curated-model removal cannot submit stale model selections.');
  } finally {
    if (browser) await browser.close();
    fs.unlinkSync(harness);
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
