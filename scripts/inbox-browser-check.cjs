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
  const task = { id: 1, project_id: 1, title: 'Improve task navigation', description: 'Task navigation', status: 'in_progress', position: 0, source: 'manual', worktree_path: null, recurrent: false };
  const errors = [];
  let agentEntries = [];
  let taskSession = { id: 'browser-session', taskId: 1, kind: 'agent', engine: 'custom', status: 'running', label: 'Agent', createdAt: new Date().toISOString() };
  let taskSocket;
  async function agent(action, body) {
    const response = await fetch(`${base}/api/agent-feedback/${action}`, { method: 'POST', headers, body: JSON.stringify(body) });
    assert.equal(response.status, 200, await response.clone().text());
    return response.json();
  }
  async function complete(key, actions = [], taskID = 1) {
    const response = await fetch(base+'/api/test-monitor/complete', {method:'POST', body:JSON.stringify({key, actions, task_id:taskID})});
    assert.equal(response.status,200,await response.clone().text());
    return response.json();
  }
  async function ask(key) {
    return agent('ask', { request_key: key, abstract: 'Compact inbox message.', summary: 'I’m improving how users move between a task and its agent terminal.', context: '**Recommendation:** focus the agent terminal when opening a question’s task.\n\n- This saves a click when replying in the terminal.\n- Keep the current tab for ordinary task navigation.\n\nNo changes have been applied yet.', question: 'Should opening a question’s task focus the agent terminal?', options: [{ id: 'focus', label: 'Focus the agent terminal' }, { id: 'preserve', label: 'Keep the current tab' }] });
  }
  try {
    const page = await browser.newPage({ viewport: { width: 1440, height: 1000 } });
    page.setDefaultTimeout(15_000);
    page.on('pageerror', e => errors.push(e.message));
    await page.routeWebSocket('**/api/sessions/*/ws', socket => { if (socket.url().includes('/browser-session/')) taskSocket = socket; socket.onMessage(() => {}); });
    await page.route('**/api/**', async route => {
      const name = new URL(route.request().url()).pathname;
      if (name.startsWith('/api/test-monitor') || name.startsWith('/api/monitor') || name.startsWith('/api/inbox') || name.startsWith('/api/agent-feedback') || name.startsWith('/api/integrations') || name.startsWith('/api/artifact-') || /^\/api\/tasks\/0\/artifact/.test(name)) return route.continue();
      const data = name === '/api/workspaces' ? [{ id: 1, name: 'Work' }]
        : name === '/api/projects' ? [{ id: 1, name: 'Kamacu', workspace_id: 1, icon_letters: 'KA', icon_color: 'blue' }]
        : name === '/api/tasks/1' ? task
        : name === '/api/sessions' && new URL(route.request().url()).searchParams.get('scope') === 'global' ? [{ id:'scratch-session', taskId:0, global:true, kind:'agent', engine:'custom', status:'running', label:'Agent', createdAt:new Date().toISOString() }]
        : name === '/api/sessions' ? [taskSession]
        : name === '/api/projects/1/tasks' ? [task]
        : name === '/api/settings' ? Object.fromEntries(['worktree_base', 'shell', 'branch_template', 'done_session_ttl', 'github_integration', 'pr_review_seed'].map(key => [key, {value: '', default: '', source: 'default'}]))
        : name === '/api/worktrees' ? {counts:{total:0,orphaned:0},projects:[]}
        : name === '/api/global' ? {root_path:'/tmp/scratchpad', agent_id:1, root_exists:true, live:{agent:1,bash:0,tmux:0}, agent:{id:1,name:'Custom',engine:'custom'}}
        : name === '/api/agents/status' ? agentEntries : [];
      return route.fulfill({ json: data });
    });
    await page.goto(base + '/inbox');
    await page.getByText('No messages waiting', { exact: true }).waitFor();
    // Membership follows terminal lifetime across activity/error transitions.
    const barEntry = (id, status, live) => ({taskId:id, projectId:1,
      sessionId:`active-${id}`, taskTitle:`Open terminal ${id}`, projectName:'Kamacu',
      source:'manual', status, live, exitCode:null, stopRequested:false, resumable:false, prNumber:null});
    agentEntries = [barEntry(11,'working',true),barEntry(12,'waiting',true),
      barEntry(13,'idle',true),barEntry(14,'error',true),barEntry(15,'exited',false),
      barEntry(16,'working',false)];
    await page.reload();
    await page.getByRole('button',{name:'Active agent sessions: 1 working, 1 waiting, 1 idle, 1 need attention — expand',exact:true}).waitFor();
    await page.getByRole('button',{name:'Expand active sessions',exact:true}).click();
    for (const id of [11,12,13,14]) {
      await page.getByRole('button',{name:`Open Open terminal ${id} in Kamacu`,exact:true}).waitFor();
    }
    for (const id of [15,16]) {
      assert.equal(await page.getByRole('button',{name:`Open Open terminal ${id} in Kamacu`,exact:true}).count(),0);
    }
    agentEntries[0].status = 'error';
    agentEntries[1].status = 'idle';
    agentEntries[2].status = 'working';
    agentEntries[3].status = 'waiting';
    await page.getByRole('button',{name:'Active agent sessions: 1 working, 1 waiting, 1 idle, 1 need attention — collapse',exact:true}).waitFor();
    // Wait for a poll containing changed counts as well as changed row states.
    agentEntries[1].status = 'error';
    await page.getByRole('button',{name:'Active agent sessions: 1 working, 1 waiting, 0 idle, 2 need attention — collapse',exact:true}).waitFor();
    for (const id of [11,12,13,14]) {
      await page.getByRole('button',{name:`Open Open terminal ${id} in Kamacu`,exact:true}).waitFor();
    }
    agentEntries = [];
    await page.getByRole('button',{name:'Collapse active sessions',exact:true}).click();
    await page.reload();
    const directory = process.env.KAMACU_INBOX_SCREENSHOTS;
    if (directory) fs.mkdirSync(directory, {recursive:true});
    async function expand(question) {
      const row = page.locator('details').filter({has:page.locator(`#question-${question.id}`)}).first();
      await row.waitFor({state:'attached'});
      if (await row.getAttribute('open') === null) await row.locator(':scope > summary').click();
    }
    for (const mode of ['dismiss', 'move_to_review', 'move_to_done', 'no-actions']) {
      const actions = mode === 'dismiss' ? ['move_to_review','move_to_done'] : mode === 'no-actions' ? [] : [mode];
      const completed = await complete('completed-'+mode, actions);
      await page.reload();
      await expand(completed.question);
      await page.getByText('What was done', {exact:true}).waitFor();
      if (directory && mode === 'dismiss') {
        await page.screenshot({path:path.join(directory,'completion-desktop.png'),fullPage:true});
        await page.setViewportSize({width:390,height:844});
        await page.screenshot({path:path.join(directory,'completion-mobile.png'),fullPage:true});
        assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), 'completion mobile overflow');
        await page.setViewportSize({width:1440,height:1000});
      }
      assert.equal(await page.getByLabel('Send the next prompt', {exact:true}).count(), 0);
      assert.equal(await page.getByRole('button', {name:'Seen',exact:true}).count(), 0);
      for (const [id,label] of [['move_to_review','Move to review and close session'],['move_to_done','Move to done and close session']]) {
        assert.equal(await page.getByRole('button',{name:label,exact:true}).count(),actions.includes(id) ? 1 : 0);
      }
      assert.equal(await page.getByRole('button',{name:'OK, exit session',exact:true}).count(),0);
      await page.getByRole('button',{name:'Dismiss',exact:true}).waitFor();
      await page.getByRole('button',{name:'Open task: Improve task navigation',exact:true}).waitFor();
      await page.getByRole('button',{name:mode === 'move_to_review' ? 'Move to review and close session' : mode === 'move_to_done' ? 'Move to done and close session' : 'Dismiss',exact:true}).click();
      await page.getByText(mode === 'move_to_review' ? 'Task moved to review. Message archived and agent session is closing.' : mode === 'move_to_done' ? 'Task moved to done. Message archived and agent session is closing.' : 'Message dismissed and archived.', {exact:true}).waitFor();
      const result = await agent('wait', {question_id:completed.question.id,wait_seconds:0});
      assert.equal(result.question.status,mode.startsWith('move_') ? 'answered' : 'dismissed');
      assert.equal(result.question.reason,mode.startsWith('move_') ? mode : '');
      assert.equal(await page.getByRole('dialog',{name:'Agent session ended',exact:true}).count(),0,'inbox exit opened the task-status dialog');
    }
    await complete('completed-sidebar',['move_to_done']);
    await page.getByRole('button',{name:'Expand inbox sidebar',exact:true}).click();
    const sidebar = page.getByRole('complementary',{name:'Inbox sidebar',exact:true});
    await sidebar.getByRole('button',{name:'Move to done and close session',exact:true}).click();
    await sidebar.getByText('Task moved to done. Message archived and agent session is closing.',{exact:true}).waitFor();
    assert.equal(await page.getByRole('dialog',{name:'Agent session ended',exact:true}).count(),0,'sidebar exit opened the task-status dialog');
    await page.getByRole('button',{name:'Collapse inbox sidebar',exact:true}).click();
    const first = await ask('options');
    const firstRow = page.locator('details').filter({has:page.locator(`#question-${first.question.id}`)}).first();
    await firstRow.waitFor({state:'attached'});
    assert.equal(await firstRow.getAttribute('open'),null,'messages should start collapsed');
    assert.equal(await page.getByRole('button',{name:'Focus the agent terminal',exact:true}).isVisible(),false);
    if(directory) await page.screenshot({path:path.join(directory,'inbox-list-desktop.png'),fullPage:true});
    await expand(first.question);
    await page.getByRole('button', { name: 'Focus the agent terminal', exact: true }).waitFor();
    await page.getByText('Recommendation:', { exact: false }).waitFor();
    if (directory) { fs.mkdirSync(directory, { recursive: true }); await page.screenshot({ path: path.join(directory, 'inbox-desktop.png'), fullPage: true }); }
    await page.setViewportSize({ width: 390, height: 844 });
    if (directory) await page.screenshot({ path: path.join(directory, 'inbox-mobile.png'), fullPage: true });
    assert.ok(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth), 'mobile page overflows horizontally');
    await page.getByRole('button', { name: 'Focus the agent terminal', exact: true }).click();
    await page.getByText('No messages waiting', { exact: true }).waitFor();
    let result = await agent('wait', { question_id: first.question.id, wait_seconds: 0 });
    assert.equal(result.question.answer, 'Focus the agent terminal');
    await page.getByRole('button', { name: 'Archived', exact: true }).click();
    await expand(first.question);
    await page.locator('article').getByText('Focus the agent terminal', { exact: true }).waitFor();
    await page.getByText('Awaiting agent', { exact: false }).waitFor();
    await agent('acknowledge', { question_id: first.question.id, revision: 1 });
    await firstRow.getByText('Received by agent', { exact: false }).waitFor();
    await page.getByRole('button', { name: /^Pending/ }).click();
    const custom = await ask('custom');
    await expand(custom.question);
    await page.getByLabel('Write your own answer').fill('Focus it only when the session is still running.');
    const customRow=page.locator('details').filter({has:page.locator(`#question-${custom.question.id}`)});
    await customRow.locator(':scope > summary').click();
    await expand(custom.question);
    assert.equal(await page.getByLabel('Write your own answer').inputValue(),'Focus it only when the session is still running.','collapse lost answer draft');
    await page.getByRole('button', { name: 'Send answer', exact: true }).click();
    await page.getByText('No messages waiting', { exact: true }).waitFor();
    const dismissed = await ask('dismiss');
    await expand(dismissed.question);
    await page.getByRole('button', { name: 'Dismiss', exact: true }).click();
    await page.getByText('No messages waiting', { exact: true }).waitFor();
    result = await agent('wait', { question_id: dismissed.question.id, wait_seconds: 0 });
    assert.equal(result.outcome, 'dismissed');
    const terminal = await ask('terminal');
    await expand(terminal.question);
    await page.getByRole('button', { name: 'Focus the agent terminal', exact: true }).waitFor();
    await agent('resolve', { question_id: terminal.question.id, revision: 1, answer: 'Answered in the terminal' });
    await page.getByText('No messages waiting', { exact: true }).waitFor();
    const navigation = await ask('navigation');
    await expand(navigation.question);
    await page.getByRole('button', { name: 'Open task: Improve task navigation' }).click();
    await page.waitForURL('**/projects/1/tasks/1?feedback_session=browser-session');
    await page.getByRole('tab', { name: 'Agent', exact: true }).waitFor();
    assert.equal(await page.getByRole('tab', { name: 'Agent', exact: true }).getAttribute('aria-selected'), 'true');
    await page.waitForFunction(() => document.activeElement?.classList.contains('xterm-helper-textarea'));
    // External exits stay on the task, even while its terminal is focused.
    // A terminal exit prompts only while the agent tab is actively viewed.
    for (const scenario of ['external', 'other-tab', 'terminal']) {
      taskSession = {...taskSession, status:'running', exitCode:undefined, exitSource:undefined, stopRequested:false};
      taskSocket = undefined;
      await page.goto(base+'/projects/1/tasks/1?feedback_session=browser-session');
      await page.bringToFront();
      await page.getByRole('tab',{name:'Agent',exact:true}).waitFor();
      await page.waitForFunction(() => document.activeElement?.classList.contains('xterm-helper-textarea'));
      assert.ok(taskSocket,'agent terminal did not connect');
      if (scenario === 'other-tab') await page.getByRole('tab',{name:'Description',exact:true}).click();
      taskSession = {...taskSession, status:'exited', exitCode:143, stopRequested:true, exitSource:scenario === 'external' ? 'external' : 'terminal'};
      const refresh = page.waitForResponse(response => new URL(response.url()).pathname === '/api/sessions' && new URL(response.url()).searchParams.get('task_id') === '1');
      taskSocket.send(Buffer.from('x'+JSON.stringify({code:143})));
      await refresh;
      await page.evaluate(() => new Promise(resolve => requestAnimationFrame(() => requestAnimationFrame(resolve))));
      if (scenario === 'terminal') {
        await page.waitForURL('**/projects/1');
        await page.getByRole('dialog',{name:'Agent session ended',exact:true}).waitFor();
        await page.getByRole('button',{name:'Keep In Progress',exact:true}).click();
      } else {
        // Let the UI render the exit, then confirm revisiting the tab stays quiet.
        await page.getByRole('tab',{name:'Agent',exact:true}).click();
        await page.getByRole('button',{name:'Reset session',exact:true}).waitFor();
        assert.ok(new URL(page.url()).pathname === '/projects/1/tasks/1','external/background exit navigated away');
        assert.equal(await page.getByRole('dialog',{name:'Agent session ended',exact:true}).count(),0);
      }
    }
    taskSession = {...taskSession, status:'running', exitCode:undefined, exitSource:undefined, stopRequested:false};
    // Exercise Scratchpad using the same real inbox and artifact APIs.
    headers['X-Kamacu-Task-ID'] = '0';
    headers['X-Kamacu-Session-ID'] = 'scratch-session';
    const scratch = await ask('scratchpad');
    await page.goto(base + '/inbox');
    await page.getByRole('button', {name:'Archived',exact:true}).click();
    await expand(navigation.question);
    await page.getByRole('button', {name:/^Pending/}).click();
    await expand(scratch.question);
    assert.equal(await page.locator('details[open]').count(),1);
    if(directory) await page.screenshot({path:path.join(directory,'inbox-multiple-mobile.png'),fullPage:true});
    await page.getByRole('button', {name:'Open Scratchpad',exact:true}).click();
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

    const scratchResult = await agent('wait',{question_id:scratch.question.id,wait_seconds:0});
    assert.equal(scratchResult.question.status,'dismissed');
    const scratchCompletion = await complete('scratch-completion', ['move_to_review','move_to_done'], 0);
    await page.goto(base+'/inbox');
    await expand(scratchCompletion.question);
    assert.equal(await page.getByRole('button',{name:/Move to (review|done) and close session/}).count(),0);
    await page.getByRole('button',{name:'Dismiss',exact:true}).waitFor();
    await page.getByRole('button',{name:'Open Scratchpad',exact:true}).click();
    await page.waitForURL('**/global?feedback_session=scratch-session');
    // Observer cards keep their identity when opened, hidden, and restored.
    const observed=await (await fetch(base+'/api/test-monitor/observe',{method:'POST'})).json();
    await page.goto(base+'/inbox');
    await expand(observed.question);
    await page.getByRole('heading',{name:'Image upload security',exact:true}).waitFor();
    await page.getByText('I require authentication and workspace membership before accepting uploads.',{exact:true}).waitFor();
    assert.equal(await page.getByRole('link',{name:'Review the PR',exact:true}).getAttribute('href'),'https://github.com/example/demo/pull/42');
    await page.getByRole('button',{name:'Open Scratchpad',exact:true}).click();
    await page.waitForURL('**/global?feedback_session=scratch-session');
    let observedState=await (await fetch(base+`/api/inbox/${observed.question.id}`)).json();
    assert.equal(observedState.status,'pending','opening an observer question dismissed it');
    await fetch(base+'/api/test-monitor/hide',{method:'POST'});
    await page.goto(base+'/inbox');
    await page.getByText('No messages waiting',{exact:true}).waitFor();
    await fetch(base+'/api/test-monitor/show',{method:'POST'});
    await expand(observed.question);
    await page.getByRole('button',{name:'2. Revise the plan.',exact:true}).waitFor();
    await page.getByLabel('Write your own answer').waitFor();
    await page.getByRole('button',{name:'1. Yes, continue.',exact:true}).click();
    await page.getByText('No messages waiting',{exact:true}).waitFor();
    observedState=await (await fetch(base+`/api/inbox/${observed.question.id}`)).json();
    assert.equal(observedState.answer,'1. Yes, continue.');
    await page.goto(base + '/settings');
    const monitoring = page.getByRole('region', {name:'Terminal monitoring'});
    await monitoring.getByText('No monitoring models yet', {exact:true}).waitFor();
    assert.equal(await page.getByRole('dialog').count(), 0);
    await monitoring.getByRole('button', {name:'Add monitoring model'}).click();
    const modelDialog = page.getByRole('dialog', {name:'Add monitoring model',exact:true});
    await modelDialog.getByLabel('Name', {exact:true}).fill('Installed Codex');
    await modelDialog.getByLabel('Connection', {exact:true}).selectOption('codex_cli');
    await modelDialog.getByRole('button', {name:'Test connection'}).click();
    await modelDialog.getByText('Connection test passed.').waitFor();
    if(directory) await page.screenshot({path:path.join(directory,'monitoring-dialog-mobile.png'),fullPage:true});
    assert.ok(await modelDialog.evaluate(el => {
      const rect = el.getBoundingClientRect();
      return rect.left >= 0 && rect.right <= window.innerWidth && rect.top >= 0 && rect.bottom <= window.innerHeight;
    }), 'monitoring dialog fits mobile viewport');
    await modelDialog.getByRole('button', {name:'Save',exact:true}).click();
    await modelDialog.waitFor({state:'hidden'});
    assert.ok(await monitoring.getByRole('button', {name:'Add monitoring model'}).evaluate(el => el === document.activeElement), 'save restores focus to Add');
    const models = monitoring.getByRole('list', {name:'Monitoring models'});
    await models.getByText('Installed Codex', {exact:true}).waitFor();
    await models.getByRole('button', {name:'Edit Installed Codex',exact:true}).click();
    const editDialog = page.getByRole('dialog', {name:'Edit monitoring model',exact:true});
    assert.equal(await editDialog.getByLabel('Name', {exact:true}).inputValue(), 'Installed Codex');
    await editDialog.getByRole('button', {name:'Cancel',exact:true}).click();
    await editDialog.waitFor({state:'hidden'});
    assert.ok(await models.getByRole('button', {name:'Edit Installed Codex',exact:true}).evaluate(el => el === document.activeElement), 'cancel restores focus to Edit');
    await monitoring.getByLabel('Global default (including Scratchpad)').selectOption({label:'Installed Codex'});
    await monitoring.getByText('Monitoring model updated.', {exact:true}).waitFor();
    await monitoring.locator('summary').filter({hasText:'Workspace models'}).click();
    const workspaceSelect=monitoring.locator('select[id="monitor-scope-1"]');
    await workspaceSelect.selectOption({label:'Installed Codex'});
    await monitoring.getByText('Monitoring model updated.', {exact:true}).waitFor();
    await workspaceSelect.selectOption('');
    await monitoring.getByText('Monitoring model updated.', {exact:true}).waitFor();
    const cfg=await (await fetch(base+'/api/monitor')).json();
    assert.ok(cfg.default_profile_id);
    assert.equal(cfg.workspaces[0].profile_id,null);
    if(directory) await page.screenshot({path:path.join(directory,'monitoring-mobile.png'),fullPage:true});
    await page.getByRole('button', {name: 'Enable Claude Code integration', exact: true}).click();
    await page.getByRole('button', {name: 'Repair Claude Code integration', exact: true}).waitFor();
    await page.getByText('Enabled · MCP registered.', {exact:true}).waitFor();
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
