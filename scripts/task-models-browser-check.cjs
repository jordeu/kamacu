// Run against Vite: node scripts/task-models-browser-check.cjs http://127.0.0.1:5187
const { chromium } = require(process.env.KAMACU_PLAYWRIGHT_PACKAGE || 'playwright');
const assert = require('node:assert/strict');

(async () => {
  const browser = await chromium.launch({ headless: true, executablePath: process.env.KAMACU_BROWSER_EXECUTABLE, args: ['--no-sandbox'] });
  try {
    const page = await browser.newPage({ viewport: { width: 1280, height: 900 } });
    page.setDefaultTimeout(10000);
    page.on('pageerror', error => console.error('Page error:', error.stack));
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    let agents = [
      { id: 1, name: 'Claude Code', engine: 'claude', command: 'claude', models: ['sonnet'], model_argument: '--model {{model}}', extra_params: '', is_system: true, is_default: true },
      { id: 2, name: 'Custom', engine: 'custom', command: 'custom-agent', models: ['small'], model_argument: '--llm={{model}}', model_discovery_command: 'custom-agent models' },
    ];
    const project = { id: 1, name: 'App', workspace_id: 1, icon_letters: 'AP', icon_color: '#3b82f6', agent_id: 1, description: '', repo_path: '/tmp/app', github_repo: null, pinned: false };
    let saves = 0;
    let created;
    let discoveryFails = false;
    await page.route('**/api/**', async route => {
      const request = route.request();
      const path = new URL(request.url()).pathname;
      if (!path.startsWith('/api/')) return route.continue();
      if (path === '/api/agents/2' && request.method() === 'DELETE') return route.fulfill({ status: 409, json: { error: 'agent is assigned to 1 task(s)' } });
      if (path === '/api/agents/discover-models') {
        assert.equal(request.postDataJSON().command, 'custom-agent');
        return discoveryFails
          ? route.fulfill({ status: 502, json: { error: 'Discovery unavailable; add models manually.' } })
          : route.fulfill({ json: { models: ['small', 'large', 'experimental'] } });
      }
      let data = [];
      if (path === '/api/agents/2' && request.method() === 'PATCH') {
        saves++;
        agents[1] = { ...agents[1], ...request.postDataJSON() };
        data = agents[1];
      } else if (path === '/api/projects/1/tasks' && request.method() === 'POST') {
        created = request.postDataJSON();
        data = { id: 1, project_id: 1, ...created };
      } else if (path === '/api/projects') data = [project];
      else if (path === '/api/projects/1') data = project;
      else if (path === '/api/workspaces') data = [{ id: 1, name: 'Work', is_default: true }];
      else if (path === '/api/agents') data = agents;
      else if (path === '/api/inbox') data = { items: [], pending_count: 0, has_more: false };
      else if (path === '/api/notifications/rules') data = [{ id: 'rules', revision: 1, status: 'active', rules: [] }];
      else if (path === '/api/notifications/status') data = { pending_count: 0, processing: null };
      else if (path === '/api/monitor') data = { profiles: [], default_profile_id: null, workspaces: [], usage: {} };
      else if (path === '/api/worktrees') data = { counts: { total: 0, orphaned: 0 }, projects: [] };
      else if (path === '/api/settings') data = Object.fromEntries(['worktree_base', 'shell', 'branch_template', 'done_session_ttl', 'github_integration', 'pr_review_seed'].map(key => [key, { value: '', default: '', source: 'default' }]));
      await route.fulfill({ json: data });
    });

    const base = process.argv[2];
    assert.ok(base, 'Pass a local Vite URL');
    await page.goto(base + '/settings?section=agents');
    await page.getByRole('button', { name: 'Edit Custom', exact: true }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByRole('button', { name: 'Discover models', exact: true }).click();
    await dialog.getByRole('button', { name: 'Add large', exact: true }).waitFor();
    assert.equal(saves, 0, 'Discovery must not save or curate automatically');
    assert.deepEqual(agents[1].models, ['small']);
    await dialog.getByLabel('Filter suggestions', { exact: true }).fill('large');
    await dialog.getByLabel('Filter suggestions', { exact: true }).press('Enter');
    assert.equal(saves, 0, 'Filtering suggestions must not submit the agent form');
    await dialog.getByRole('button', { name: 'Add large', exact: true }).click();
    assert.equal(await dialog.getByRole('button', { name: 'Remove large', exact: true }).count(), 1);
    await dialog.getByLabel('Filter suggestions', { exact: true }).fill('');
    await dialog.getByLabel('Command', { exact: true }).fill('other-agent');
    assert.equal(await dialog.getByRole('button', { name: 'Add experimental', exact: true }).count(), 0, 'Suggestions from a different command must disappear');
    await dialog.getByLabel('Command', { exact: true }).fill('custom-agent');
    await dialog.getByRole('button', { name: 'Remove small', exact: true }).click();
    await dialog.getByRole('textbox', { name: 'Model ID', exact: true }).fill('manual-model');
    await dialog.getByRole('button', { name: 'Add model', exact: true }).click();
    await dialog.getByLabel('Model argument', { exact: true }).fill('-m {{model}}');
    await dialog.getByRole('button', { name: 'Save', exact: true }).click();
    await dialog.waitFor({ state: 'hidden' });
    assert.equal(saves, 1);
    assert.deepEqual(agents[1].models, ['large', 'manual-model']);
    assert.equal(agents[1].model_argument, '-m {{model}}');

    await page.getByRole('button', { name: 'Edit Custom', exact: true }).click();
    assert.equal(await dialog.getByRole('button', { name: 'Remove large', exact: true }).count(), 1, 'Saved list must reload');
    discoveryFails = true;
    await dialog.getByRole('button', { name: 'Discover models', exact: true }).click();
    await dialog.getByRole('alert').waitFor();
    assert.equal(await dialog.getByRole('button', { name: 'Remove large', exact: true }).count(), 1, 'Discovery failure must retain the curated list');
    await dialog.getByRole('button', { name: 'Remove large', exact: true }).click();
    await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
    assert.deepEqual(agents[1].models, ['large', 'manual-model'], 'Cancel must discard edits');

    await page.getByRole('button', { name: 'Edit Claude Code', exact: true }).click();
    assert.equal(await dialog.getByLabel('Discovery command (optional override)', { exact: true }).count(), 1, 'Built-in agents must support a provider-specific discovery override');
    await dialog.getByRole('button', { name: 'Cancel', exact: true }).click();
    await page.getByRole('button', { name: 'Delete Custom', exact: true }).click();
    const confirmation = page.getByRole('alertdialog');
    await confirmation.getByRole('button', { name: 'Delete agent', exact: true }).click();
    await confirmation.getByRole('alert').waitFor();
    assert.match(await confirmation.getByRole('alert').innerText(), /assigned to 1 task/);
    assert.equal(await confirmation.isVisible(), true, 'A failed deletion must keep the explanation visible');
    await confirmation.getByRole('button', { name: 'Cancel', exact: true }).click();

    await page.goto(base + '/projects/1');
    await page.getByRole('button', { name: 'New task', exact: true }).click();
    assert.equal(await page.getByLabel('Agent', { exact: true }).inputValue(), '', 'Project agent should be the default');
    assert.equal(await page.getByLabel('Model', { exact: true }).locator('option[value="sonnet"]').count(), 1);
    await page.getByLabel('Model', { exact: true }).selectOption('sonnet');
    await page.getByLabel('Agent', { exact: true }).selectOption('2');
    assert.equal(await page.getByLabel('Model', { exact: true }).inputValue(), '', 'Changing agent must clear the old model');
    assert.equal(await page.getByLabel('Model', { exact: true }).locator('option[value="sonnet"]').count(), 0);
    await page.getByRole('button', { name: 'Cancel', exact: true }).click();
    await page.getByRole('button', { name: 'New task', exact: true }).click();
    assert.equal(await page.getByLabel('Agent', { exact: true }).inputValue(), '', 'A new dialog should start with the project agent');
    await page.getByLabel('Agent', { exact: true }).selectOption('2');
    await page.getByLabel('Model', { exact: true }).selectOption('large');
    await page.getByPlaceholder('Task title', { exact: true }).fill('Selected model task');
    await page.getByRole('button', { name: 'Create task', exact: true }).click();
    await page.getByRole('dialog').waitFor({ state: 'hidden' });
    assert.deepEqual(created, { title: 'Selected model task', description: '', agent_id: 2, model: 'large' });
    assert.deepEqual(errors, []);
    console.log('Verified curated model discovery, manual add/remove, save/cancel, failure handling, agent defaults, model reset, and task payload.');
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
