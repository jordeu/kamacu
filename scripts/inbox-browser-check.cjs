// KAMACU_BROWSER_TEST=1 go test ./internal/api -run '^TestInboxBrowser$'
// Uses the existing KAMACU_PLAYWRIGHT_PACKAGE / KAMACU_BROWSER_EXECUTABLE knobs.
const { chromium } = require(process.env.KAMACU_PLAYWRIGHT_PACKAGE || 'playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
(async () => {
  const base = process.argv[2];
  const browser = await chromium.launch({ headless: true, executablePath: process.env.KAMACU_BROWSER_EXECUTABLE, args: ['--no-sandbox'] });
  const headers = { 'X-Kamacu-Token': 'test-token', 'X-Kamacu-Task-ID': '1', 'X-Kamacu-Session-ID': 'browser-session' };
  const task = { id: 1, project_id: 1, title: 'Improve task navigation', description: 'Task navigation', status: 'todo', position: 0, source: 'manual', worktree_path: null, recurrent: false };
  const errors = [];
  async function agent(action, body) {
    const response = await fetch(`${base}/api/agent-feedback/${action}`, { method: 'POST', headers, body: JSON.stringify(body) });
    assert.equal(response.status, 200, await response.clone().text());
    return response.json();
  }
  async function ask(key) {
    return agent('ask', { request_key: key, summary: 'I’m improving how users move between a task and its agent terminal.', context: '**Recommendation:** focus the agent terminal when opening a question’s task.\n\n- This saves a click when replying in the terminal.\n- Keep the current tab for ordinary task navigation.\n\nNo changes have been applied yet.', question: 'Should opening a question’s task focus the agent terminal?', options: [{ id: 'focus', label: 'Focus the agent terminal' }, { id: 'preserve', label: 'Keep the current tab' }] });
  }
  try {
    const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } });
    page.setDefaultTimeout(15_000);
    page.on('pageerror', e => errors.push(e.message));
    await page.routeWebSocket('**/api/sessions/*/ws', socket => { socket.onMessage(() => {}); });
    await page.route('**/api/**', async route => {
      const name = new URL(route.request().url()).pathname;
      if (name.startsWith('/api/inbox') || name.startsWith('/api/agent-feedback') || name.startsWith('/api/integrations') || name.startsWith('/api/artifact-') || /^\/api\/tasks\/0\/artifact/.test(name)) return route.continue();
      const data = name === '/api/workspaces' ? [{ id: 1, name: 'Work' }]
        : name === '/api/projects' ? [{ id: 1, name: 'Kamacu', workspace_id: 1, icon_letters: 'KA', icon_color: 'blue' }]
        : name === '/api/tasks/1' ? task
        : name === '/api/sessions' && new URL(route.request().url()).searchParams.get('scope') === 'global' ? [{ id:'scratch-session', taskId:0, global:true, kind:'agent', engine:'custom', status:'running', label:'Agent', createdAt:new Date().toISOString() }]
        : name === '/api/sessions' ? [{ id: 'browser-session', taskId: 1, kind: 'agent', engine: 'custom', status: 'running', label: 'Agent', createdAt: new Date().toISOString() }]
        : name === '/api/settings' ? Object.fromEntries(['worktree_base', 'shell', 'branch_template', 'done_session_ttl', 'github_integration', 'pr_review_seed'].map(key => [key, {value: '', default: '', source: 'default'}]))
        : name === '/api/worktrees' ? {counts:{total:0,orphaned:0},projects:[]}
        : name === '/api/global' ? {root_path:'/tmp/scratchpad', agent_id:1, root_exists:true, live:{agent:1,bash:0,tmux:0}, agent:{id:1,name:'Custom',engine:'custom'}}
        : name === '/api/agents/status' ? [] : [];
      return route.fulfill({ json: data });
    });
    await page.goto(base + '/inbox');
    await page.getByText('No questions waiting', { exact: true }).waitFor();
    const first = await ask('options');
    await page.getByRole('button', { name: 'Focus the agent terminal', exact: true }).waitFor();
    await page.getByText('Recommendation:', { exact: false }).waitFor();
    const directory = process.env.KAMACU_INBOX_SCREENSHOTS;
    if (directory) { fs.mkdirSync(directory, { recursive: true }); await page.screenshot({ path: path.join(directory, 'inbox-desktop.png'), fullPage: true }); }
    await page.setViewportSize({ width: 390, height: 844 });
    if (directory) await page.screenshot({ path: path.join(directory, 'inbox-mobile.png'), fullPage: true });
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), 'mobile page overflows horizontally');
    await page.getByRole('button', { name: 'Focus the agent terminal', exact: true }).click();
    await page.getByText('No questions waiting', { exact: true }).waitFor();
    let result = await agent('wait', { question_id: first.question.id, wait_seconds: 0 });
    assert.equal(result.question.answer, 'Focus the agent terminal');
    await page.getByRole('button', { name: 'Archived', exact: true }).click();
    await page.getByText('Focus the agent terminal', { exact: true }).waitFor();
    await page.getByText('Awaiting agent', { exact: false }).waitFor();
    await agent('acknowledge', { question_id: first.question.id, revision: 1 });
    await page.getByText('Received by agent', { exact: false }).waitFor();
    await page.getByRole('button', { name: /^Pending/ }).click();
    await ask('custom');
    await page.getByLabel('Write your own answer').fill('Focus it only when the session is still running.');
    await page.getByRole('button', { name: 'Send answer', exact: true }).click();
    await page.getByText('No questions waiting', { exact: true }).waitFor();
    const dismissed = await ask('dismiss');
    await page.getByRole('button', { name: 'Dismiss', exact: true }).click();
    await page.getByText('No questions waiting', { exact: true }).waitFor();
    result = await agent('wait', { question_id: dismissed.question.id, wait_seconds: 0 });
    assert.equal(result.outcome, 'dismissed');
    const terminal = await ask('terminal');
    await page.getByRole('button', { name: 'Focus the agent terminal', exact: true }).waitFor();
    await agent('resolve', { question_id: terminal.question.id, revision: 1, answer: 'Answered in the terminal' });
    await page.getByText('No questions waiting', { exact: true }).waitFor();
    await ask('navigation');
    await page.getByRole('link', { name: 'Open task: Improve task navigation' }).click();
    await page.waitForURL('**/projects/1/tasks/1?feedback_session=browser-session');
    await page.getByRole('tab', { name: 'Agent', exact: true }).waitFor();
    assert.equal(await page.getByRole('tab', { name: 'Agent', exact: true }).getAttribute('aria-selected'), 'true');
    await page.waitForFunction(() => document.activeElement?.classList.contains('xterm-helper-textarea'));
    // Exercise Scratchpad using the same real inbox and artifact APIs.
    headers['X-Kamacu-Task-ID'] = '0';
    headers['X-Kamacu-Session-ID'] = 'scratch-session';
    const scratch = await ask('scratchpad');
    await page.goto(base + '/inbox');
    await page.getByRole('link', {name:'Open Scratchpad',exact:true}).click();
    await page.waitForURL('**/global?feedback_session=scratch-session');
    await page.waitForFunction(() => document.activeElement?.classList.contains('xterm-helper-textarea'));
    const html = '<h1>Scratchpad preview</h1><script>document.body.dataset.ready="yes"</script>';
    const begin = await fetch(base+'/api/tasks/0/artifact-uploads',{method:'POST',headers,body:JSON.stringify({title:'Scratchpad preview',filename:'preview.html',size_bytes:Buffer.byteLength(html)})});
    assert.equal(begin.status,201,await begin.clone().text());
    const upload = await begin.json();
    const put = await fetch(base+upload.upload_url,{method:'PUT',headers:upload.headers,body:html});
    assert.equal(put.status,204);
    const finalized = await fetch(base+`/api/tasks/0/artifact-uploads/${upload.upload_id}/finalize`,{method:'POST',headers});
    assert.equal(finalized.status,200,await finalized.clone().text());
    await page.getByRole('tab',{name:'Scratchpad preview',exact:true}).click();
    await page.frameLocator('iframe').getByRole('heading',{name:'Scratchpad preview',exact:true}).waitFor();
    await page.setViewportSize({width:1440,height:1000});
    if(directory) await page.screenshot({path:path.join(directory,'scratchpad-desktop.png'),fullPage:true});
    await page.setViewportSize({width:390,height:844});
    if(directory) await page.screenshot({path:path.join(directory,'scratchpad-mobile.png'),fullPage:true});
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth<=window.innerWidth),'Scratchpad mobile overflow');
    await page.getByRole('button',{name:'Remove Scratchpad preview',exact:true}).click();
    await page.getByRole('button',{name:'Remove artifact',exact:true}).click();
    await page.getByRole('tab',{name:'Scratchpad preview',exact:true}).waitFor({state:'detached'});
    assert.equal(await page.getByRole('tab',{name:'Agent',exact:true}).getAttribute('aria-selected'),'true');
    await agent('resolve',{question_id:scratch.question.id,revision:1,answer:'Answered through Scratchpad terminal'});
    const scratchResult = await agent('wait',{question_id:scratch.question.id,wait_seconds:0});
    assert.equal(scratchResult.question.status,'answered');
    await page.goto(base + '/settings');
    await page.getByRole('button', {name: 'Enable Claude Code integration', exact: true}).click();
    await page.getByRole('button', {name: 'Repair Claude Code integration', exact: true}).waitFor();
    await page.getByText('Enabled · MCP registered and task skill available.', {exact:true}).waitFor();
    await page.setViewportSize({width:1440,height:1000});
    if (directory) await page.screenshot({path:path.join(directory,'integration-desktop.png'),fullPage:true});
    await page.setViewportSize({width:390,height:844});
    if (directory) await page.screenshot({path:path.join(directory,'integration-mobile.png'),fullPage:true});
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),'integration mobile overflow');
    await page.getByRole('button', {name: 'Repair Claude Code integration', exact:true}).click();
    await page.getByRole('button', {name: 'Remove Claude Code integration', exact:true}).click();
    await page.getByRole('button', {name: 'Enable Claude Code integration', exact:true}).waitFor();
    assert.deepEqual(errors, []);
    console.log('Inbox: option and free-text answers, dismissal, terminal resolution, acknowledgment, mobile layout, task and Scratchpad navigation, Scratchpad artifact upload/view/removal, and integration enable/repair/remove passed.');
  } finally { await browser.close(); }
})().catch(error => { console.error(error); process.exit(1); });
