// Run against a local Vite server: node scripts/task-pr-links-browser-check.cjs http://127.0.0.1:5179
// KAMACU_PLAYWRIGHT_PACKAGE and KAMACU_BROWSER_EXECUTABLE override local browser tooling.
const { chromium } = require(process.env.KAMACU_PLAYWRIGHT_PACKAGE || 'playwright');
const assert = require('node:assert/strict');
(async () => {
 const browser = await chromium.launch({headless:true,executablePath:process.env.KAMACU_BROWSER_EXECUTABLE,args:['--no-sandbox']});
 try {
  const page = await browser.newPage({ viewport:{width:1440,height:1000},hasTouch:true });
  page.setDefaultTimeout(10000);
  const errors = [];
  const base = process.argv[2];
  assert.ok(base, 'Pass the local Vite server URL');
  page.on('pageerror', error => errors.push(error.message));

  const url = 'https://github.com/acme/app/pull/12';
  const task = {id:1,project_id:1,title:'Show implementation PR links',description:'Task description',status:'in_progress',source:'manual',position:0,recurrent:false,branch:'task/pr-links',worktree_path:'/tmp/worktree',worktree_error:null,pr_number:null,pr_links:[{task_id:1,repository:'acme/app',number:12,url,role:'implementation'},{task_id:1,repository:'acme/app',number:99,url:'https://github.com/acme/app/pull/99',role:'incidental'}]};
  const unlinked = {...task,id:2,title:'Task without a PR',pr_links:[]};
  await page.route('https://github.com/**', route => route.fulfill({body:'GitHub PR test destination'}));
  await page.route('**/api/**', route => {
   const p = new URL(route.request().url()).pathname;
   if (!p.startsWith('/api/')) return route.continue();
   if (p === '/api/events' || p === '/api/pending') return route.fulfill({contentType:'text/event-stream',body:': mock stream\n\n'});
   const data = p === '/api/projects' ? [{id:1,name:'Kamacu',workspace_id:1,icon_letters:'KA',icon_color:'blue',agent_id:1}]
    : p === '/api/workspaces' ? [{id:1,name:'Work'}]
    : p === '/api/agents' ? [{id:1,name:'Custom',engine:'custom'}]
    : p === '/api/projects/1/tasks' ? [task,unlinked]
    : p === '/api/tasks/1' ? task
    : p === '/api/settings' ? {github_integration:{value:'off'}}
    : p === '/api/inbox/count' ? {count:0}
    : [];
   return route.fulfill({json:data});
  });
  await page.goto(base + '/projects/1');
  const link = page.getByRole('link',{name:'Open acme/app PR #12 on GitHub (opens in a new tab)'});
  await link.waitFor();
  assert.equal(await page.getByRole('link',{name:/PR #99/}).count(),0);
  const box = await link.boundingBox();
  await page.mouse.move(box.x + box.width / 2, box.y + box.height / 2);
  await page.mouse.down();
  await page.mouse.move(box.x + box.width / 2 + 30, box.y + box.height / 2 + 30, {steps:5});
  assert.equal(await page.getByText(task.title,{exact:true}).count(), 1, 'Pressing and moving on a PR link must not start a card drag');
  await page.mouse.up();
  // TouchSensor starts a drag after a 250 ms hold, even without moving.
  const cdp = await page.context().newCDPSession(page);
  await cdp.send('Input.dispatchTouchEvent', {type:'touchStart',touchPoints:[{x:box.x+box.width/2,y:box.y+box.height/2}]});
  await page.waitForTimeout(350);
  assert.equal(await page.getByText(task.title,{exact:true}).count(), 1, 'Holding a PR link must not start a touch drag');
  await cdp.send('Input.dispatchTouchEvent', {type:'touchCancel',touchPoints:[]});
  await cdp.detach();
  // The title still starts a normal card drag; only the PR links opt out.
  const title = page.getByText(task.title,{exact:true});
  const titleBox = await title.boundingBox();
  await page.mouse.move(titleBox.x + 20, titleBox.y + titleBox.height / 2);
  await page.mouse.down();
  await page.mouse.move(titleBox.x + 50, titleBox.y + titleBox.height / 2 + 30, {steps:5});
  assert.equal(await title.count(), 2, 'The task title must still start a normal card drag');
  await page.keyboard.press('Escape');
  await page.mouse.up();
  await page.locator('.cursor-grabbing').waitFor({state:'hidden'});
  let popupPromise = page.waitForEvent('popup');
  await link.click();
  let popup = await popupPromise;
  await popup.waitForLoadState();
  assert.equal(popup.url(),url);
  assert.equal(new URL(page.url()).pathname,'/projects/1');
  await popup.close();
  // Keyboard activation also opens GitHub without activating the sortable card.
  await link.focus();
  popupPromise = page.waitForEvent('popup');
  await page.keyboard.press('Enter');
  popup = await popupPromise;
  await popup.close();
  assert.equal(new URL(page.url()).pathname,'/projects/1');
  // Compact column cards keep the latest association visible, regardless of PR number.
  task.pr_links[0].observed_at='2026-11-14T10:00:00Z';
  task.pr_links[0].linked_at='2026-11-14T10:00:00Z';
  task.pr_links.push({task_id:1,repository:'acme/app',number:47,url:'https://github.com/acme/app/pull/47',role:'implementation',linked_at:'2026-11-13T10:00:00Z',observed_at:'2026-11-15T10:00:00Z'});
  await page.reload();await link.waitFor();
  const older=page.getByRole('link',{name:'Open acme/app PR #47 on GitHub (opens in a new tab)'});
  const expand=page.getByRole('button',{name:'Show 1 more linked pull request'});
  await expand.waitFor();assert.equal(await older.count(),0);
  const collapsedHeight=await page.getByRole('button',{name:task.title,exact:true}).evaluate(el=>el.getBoundingClientRect().height);
  await expand.click();await older.waitFor();
  assert.equal(new URL(page.url()).pathname,'/projects/1','Expanding does not navigate');
  assert.ok(await page.getByRole('button',{name:task.title,exact:true}).evaluate(el=>el.getBoundingClientRect().height)>collapsedHeight);
  const collapse=page.getByRole('button',{name:'Hide additional linked pull requests'});
  await collapse.focus();await page.keyboard.press('Enter');await older.waitFor({state:'hidden'});
  await expand.focus();await page.keyboard.press('Space');await older.waitFor();
  assert.equal(await page.getByText(task.title,{exact:true}).count(),1,'Keyboard expansion does not drag');
  await collapse.click();await older.waitFor({state:'hidden'});
  await page.setViewportSize({width:390,height:844});
  await expand.tap();await older.waitFor();
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
  await collapse.tap();await older.waitFor({state:'hidden'});
  await page.setViewportSize({width:1440,height:1000});
  await page.getByText(task.title,{exact:true}).click();
  await page.waitForURL('**/projects/1/tasks/1');
  await page.getByText('task/pr-links',{exact:true}).first().waitFor();
  await link.waitFor();
  await older.waitFor();
  assert.equal(await page.getByRole('button',{name:/more linked pull|Hide additional linked/}).count(),0,'Task page shows the full PR list');
  popupPromise = page.waitForEvent('popup');
  await link.click();
  popup = await popupPromise;
  await popup.waitForLoadState();
  assert.equal(popup.url(),url);
  await popup.close();
  await page.setViewportSize({width:390,height:844});
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
  assert.deepEqual(errors,[]);
  console.log('PASS: latest PR selection, expand/collapse with mouse/keyboard/touch, compact card height, full task-page list, mouse/touch drag isolation, GitHub popups, rejected links hidden, card navigation, mobile overflow.');
 } finally { await browser.close(); }
})().catch(error=>{console.error(error);process.exit(1)});
