// Run against Vite: node scripts/task-agent-edit-browser-check.cjs http://127.0.0.1:5187
const { chromium } = require(process.env.KAMACU_PLAYWRIGHT_PACKAGE || 'playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

(async () => {
  const filename = `__task-agent-edit-${process.pid}.tsx`;
  const harness = path.join(__dirname, '../web/src', filename);
  let task = { id: 1, project_id: 1, title: 'Existing task', description: 'Original description', source: 'manual', agent_id: 1, model: 'sonnet' };
  const agents = [
    { id: 1, name: 'Claude', engine: 'claude', models: ['sonnet', 'opus'], model_argument: '--model {{model}}' },
    { id: 2, name: 'Custom', engine: 'custom', models: ['small', 'large'], model_argument: '--model {{model}}' },
  ];
  fs.writeFileSync(harness, `
import React from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { useTask } from './api/queries';
import { DescriptionTab } from './components/task/DescriptionTab';
import './index.css';
const client = new QueryClient({defaultOptions: {queries: {staleTime: Infinity, retry: false}}});
client.setQueryData(['agents'], ${JSON.stringify(agents)});
client.setQueryData(['projects'], [{id:1, agent_id:1}]);
client.setQueryData(['task',1], ${JSON.stringify(task)});
client.setQueryData(['sessions',1], []);
Object.assign(window, { agentEditReview: {
  task: (value: unknown) => client.setQueryData(['task',1], value),
  agents: (value: unknown) => client.setQueryData(['agents'], value),
  sessions: (value: unknown) => client.setQueryData(['sessions',1], value),
}});
function Harness() {
 const {data} = useTask(1);
 return <DescriptionTab task={data!} projectId={1} readOnlySource={data!.source === 'github_pr' ? 'GitHub review description' : undefined} />;
}
createRoot(document.getElementById('root')!).render(<QueryClientProvider client={client}><Harness /></QueryClientProvider>);
`, { flag: 'wx' });
  let browser;
  try {
    browser = await chromium.launch({ headless: true, executablePath: process.env.KAMACU_BROWSER_EXECUTABLE, args: ['--no-sandbox'] });
    const page = await browser.newPage();
    page.setDefaultTimeout(10000);
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    const requests = [];
    let rejectNext = false;
    let sessions = [];
    let projectDefault = 1;
    await page.route('**/__task-agent-edit-check', route => route.fulfill({ contentType: 'text/html', body: `
<html><head><script type="module">
import { injectIntoGlobalHook } from '/@react-refresh';
injectIntoGlobalHook(window); window.$RefreshReg$ = () => {}; window.$RefreshSig$ = () => type => type;
</script></head><body><div id="root"></div><script type="module" src="/src/${filename}"></script></body></html>` }));
    await page.route('**/api/**', async route => {
      const request = route.request(), pathname = new URL(request.url()).pathname;
      if (!pathname.startsWith('/api/')) return route.continue();
      if (pathname === '/api/tasks/1') {
        if (request.method() === 'PATCH') {
          const body = request.postDataJSON();
          requests.push(body);
          if (rejectNext) {
            rejectNext = false;
            return route.fulfill({ status: 409, json: { error: 'stop the running agent before changing its agent or model' } });
          }
          const { expected_agent_id, expected_model, ...changes } = body;
          if ((Object.hasOwn(body, 'expected_agent_id') && expected_agent_id !== (task.agent_id ?? null)) ||
              (Object.hasOwn(body, 'expected_model') && expected_model !== (task.model ?? ''))) {
            return route.fulfill({ status: 409, json: { error: 'agent settings changed; reload the saved settings before trying again' } });
          }
          task = { ...task, ...changes };
          if (task.agent_id == null && task.model) task.agent_id = projectDefault;
        }
        return route.fulfill({ json: task });
      }
      return route.fulfill({ json: pathname === '/api/sessions' ? sessions : [] });
    });
    await page.goto(process.argv[2] + '/__task-agent-edit-check');
    const panel = page.getByRole('region', { name: 'Task agent configuration' });
    const agent = panel.getByLabel('Agent', { exact: true });
    const model = panel.getByLabel('Model', { exact: true });
    const save = panel.getByRole('button', { name: 'Save agent and model', exact: true });
    const waitSaved = async () => { await save.waitFor({ state: 'hidden' }); };
    assert.equal(await agent.inputValue(), '1');
    assert.equal(await model.inputValue(), 'sonnet');
    await agent.selectOption('2');
    assert.equal(await model.inputValue(), '', 'Changing the agent must clear the old model');
    await model.selectOption('large');
    await save.click();
    await waitSaved();
    assert.deepEqual(requests.at(-1), { agent_id: 2, model: 'large', expected_agent_id: 1, expected_model: 'sonnet' });
    assert.equal(task.description, 'Original description');
    await agent.selectOption('1');
    await model.selectOption('opus');
    await panel.getByRole('button', { name: 'Cancel', exact: true }).click();
    assert.equal(await agent.inputValue(), '2');
    assert.equal(await model.inputValue(), 'large');
    assert.equal(requests.length, 1, 'Cancel must not save');
    // Refreshing an existing draft must not silently overwrite another edit.
    await model.selectOption('small');
    task = { ...task, agent_id: 1, model: 'opus' };
    await page.evaluate(value => window.agentEditReview.task(value), task);
    await panel.getByText(/Agent settings changed elsewhere/).waitFor();
    assert.equal(await save.isDisabled(), true);
    await panel.getByRole('button', { name: 'Cancel', exact: true }).click();
    assert.equal(await agent.inputValue(), '1');
    assert.equal(await model.inputValue(), 'opus');
    task = { ...task, agent_id: 2, model: 'large' };
    await page.evaluate(value => window.agentEditReview.task(value), task);
    await page.waitForFunction(() => document.getElementById('description-model').value === 'large');
    // A server-side edit can arrive after the UI last fetched its task.
    await model.selectOption('small');
    task = { ...task, agent_id: 1, model: 'opus' };
    await save.click();
    await panel.getByText(/Agent settings changed elsewhere/).waitFor();
    assert.equal(await save.isDisabled(), true);
    assert.equal(task.agent_id, 1, 'A stale save must preserve the other editor’s agent');
    await panel.getByRole('button', { name: 'Cancel', exact: true }).click();
    assert.equal(await model.inputValue(), 'opus');
    task = { ...task, agent_id: 2, model: 'large' };
    await page.evaluate(value => window.agentEditReview.task(value), task);
    await page.waitForFunction(() => document.getElementById('description-model').value === 'large');
    rejectNext = true;
    await model.selectOption('small');
    await save.click();
    await panel.getByRole('alert').waitFor();
    assert.equal(await model.inputValue(), 'small', 'Failed saves must retain the draft');
    assert.equal(task.model, 'large');
    await save.click();
    await waitSaved();
    assert.equal(task.model, 'small');
    // A draft model disappearing from curation must not silently save the default.
    await model.selectOption('large');
    agents[1].models = ['small'];
    await page.evaluate(value => window.agentEditReview.agents(value), agents);
    await panel.getByText(/The selected model is no longer available/).waitFor();
    assert.equal(await model.inputValue(), 'large');
    assert.equal(await save.isDisabled(), true);
    await model.selectOption('');
    assert.equal(await save.isEnabled(), true);
    await panel.getByRole('button', { name: 'Cancel', exact: true }).click();
    // A saved model still in the curated list must remain visible even when
    // its launch argument is removed, and users must be able to clear it.
    agents[1].models = ['small', 'large'];
    agents[1].model_argument = '';
    await page.evaluate(value => window.agentEditReview.agents(value), agents);
    await model.getByRole('option', { name: 'small (saved)', exact: true }).waitFor({ state: 'attached' });
    assert.equal(await model.isEnabled(), true);
    assert.equal(await model.inputValue(), 'small');
    assert.match(await model.locator('option:checked').innerText(), /small.*saved/);
    await model.selectOption('');
    await save.click();
    await waitSaved();
    assert.equal(task.model, '');
    // Running bash tabs do not block configuration, but running agents do.
    sessions = [{ id: 'bash', kind: 'bash', status: 'running' }];
    await page.evaluate(value => window.agentEditReview.sessions(value), sessions);
    assert.equal(await agent.isEnabled(), true);
    sessions = [{ id: 'agent', kind: 'agent', status: 'running' }];
    await page.evaluate(value => window.agentEditReview.sessions(value), sessions);
    await page.waitForFunction(() => document.getElementById('description-agent').matches(':disabled'));
    assert.equal(await agent.isDisabled(), true);
    await page.getByRole('button', { name: 'Edit', exact: true }).click();
    await page.getByRole('textbox').fill('Description can still change');
    await page.getByRole('button', { name: 'Save description', exact: true }).click();
    await page.getByRole('button', { name: 'Edit', exact: true }).waitFor();
    assert.deepEqual(requests.at(-1), { description: 'Description can still change' });
    sessions = [];
    await page.evaluate(value => window.agentEditReview.sessions(value), sessions);
    task = { ...task, source: 'github_pr' };
    await page.evaluate(value => window.agentEditReview.task(value), task);
    await page.getByText('GitHub review description', { exact: true }).waitFor();
    assert.equal(await page.getByRole('button', { name: 'Edit', exact: true }).count(), 0);
    await agent.selectOption('1');
    await model.selectOption('opus');
    await save.click();
    await waitSaved();
    assert.equal(task.agent_id, 1);
    assert.equal(task.model, 'opus');
    assert.equal(await page.getByText('GitHub review description', { exact: true }).count(), 1);
    // Model choices pin the displayed agent, even if its project default changes
    // on the server before the editor receives the new project configuration.
    task = { ...task, agent_id: null, model: '' };
    await page.evaluate(value => window.agentEditReview.task(value), task);
    await page.waitForFunction(() => document.getElementById('description-agent').value === '');
    await model.selectOption('opus');
    projectDefault = 2;
    await save.click();
    await waitSaved();
    assert.equal(requests.at(-1).agent_id, 1, 'A selected model must pin the displayed agent rather than resolve a new project default');
    assert.equal(task.agent_id, 1);
    assert.deepEqual(errors, []);
    console.log('Verified Description-tab agent/model save, cancel, failed-save recovery, removed models, active-session guard, and read-only PR descriptions.');
  } finally {
    if (browser) await browser.close();
    fs.unlinkSync(harness);
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
