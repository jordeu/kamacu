// Run with a local Vite server URL; browser tooling overrides match the other browser checks.
const { chromium } = require(process.env.KAMACU_PLAYWRIGHT_PACKAGE || 'playwright');
const assert = require('node:assert/strict');
(async () => {
 const base=process.argv[2];assert.ok(base,'Pass a local Vite server URL');
 const browser=await chromium.launch({headless:true,executablePath:process.env.KAMACU_BROWSER_EXECUTABLE,args:['--no-sandbox']});
 try {
  const page=await browser.newPage({viewport:{width:1440,height:1000},hasTouch:true});page.setDefaultTimeout(10000);
  const errors=[];page.on('pageerror',e=>errors.push(e.message));let statusReads=0;
  const link={task_id:1,repository:'acme/app',number:12,url:'https://github.com/acme/app/pull/12',role:'implementation'};
  const task={id:1,project_id:1,title:'Show implementation PR links',description:'Task description',status:'in_progress',source:'manual',position:0,recurrent:false,branch:'task/pr-links',worktree_path:'/tmp/worktree',worktree_error:null,pr_number:null,pr_links:[link]};
  let snapshot={repository:'acme/app',number:12,lifecycle:'open',head:'a84c190',humans:{state:'approved',reviewers:[{login:'maria',state:'approved'}]},bots:{state:'comments',reviewers:[{login:'reviewbot',state:'comments',findings:2,detail:'Unresolved bot review threads'}]},ci:{state:'pass',jobs:[{name:'Unit tests',state:'pass'}]},fetched_at:'2026-11-13T10:00:00Z',stale:false};
  await page.route('**/api/**',route=>{
   const p=new URL(route.request().url()).pathname;if(!p.startsWith('/api/'))return route.continue();
   if(p==='/api/events'||p==='/api/pending')return route.fulfill({contentType:'text/event-stream',body:': mock stream\n\n'});
   if(p==='/api/projects/1/task-pr-statuses'){statusReads++;return route.fulfill({json:{statuses:[snapshot]}})}
   const data=p==='/api/projects'?[{id:1,name:'Kamacu',workspace_id:1,icon_letters:'KA',icon_color:'blue',agent_id:1}]
    :p==='/api/workspaces'?[{id:1,name:'Work'}]:p==='/api/agents'?[{id:1,name:'Custom',engine:'custom'}]
    :p==='/api/projects/1/tasks'?[task,{...task,id:2,title:'Another linked task'}]
    :p==='/api/tasks/1'?task:p==='/api/settings'?{github_integration:{value:'on'}}
    :p==='/api/inbox/count'?{count:0}:[];
   return route.fulfill({json:data});
  });
  const status=()=>page.locator('button[aria-label*="Show PR status overview"]').first();
  const overview=()=>page.getByRole('dialog',{name:'acme/app PR 12 status overview'});
  await page.goto(base+'/projects/1');await status().waitFor();
  await page.waitForFunction(()=>document.querySelector('button[aria-label*="Human approved · CI passing"]'));
  assert.equal(statusReads,1,'Cards share one project-wide status request');
  assert.match(await status().getAttribute('aria-label'),/Human approved · CI passing. Bots: Review comments/);
  assert.equal(await status().locator('.lucide-message-square').count(),1);
  await status().hover();await overview().waitFor();
  for(const name of ['CI checks','Human reviews','Bot feedback'])await overview().getByRole('region',{name}).waitFor();
  assert.match(await overview().innerText(),/@maria/);assert.match(await overview().innerText(),/@reviewbot/);assert.match(await overview().innerText(),/2 open findings/);
  await overview().hover();assert.equal(await overview().isVisible(),true);
  if(process.env.KAMACU_BROWSER_SCREENSHOTS)await page.screenshot({path:process.env.KAMACU_BROWSER_SCREENSHOTS+'-board.png'});
  await page.keyboard.press('Escape');await overview().waitFor({state:'hidden'});
  // Escape while reading the portal must not suppress the next real hover.
  await status().hover();await overview().waitFor();
  await page.keyboard.press('Escape');await overview().waitFor({state:'hidden'});
  await status().focus();await overview().waitFor();await page.keyboard.press('Escape');await overview().waitFor({state:'hidden'});
  await status().click();await page.mouse.move(1380,20);assert.equal(await overview().isVisible(),true,'Click pins the overview');
  await status().click();await overview().waitFor({state:'hidden'});
  await status().click();await page.mouse.click(1350,70);await overview().waitFor({state:'hidden'});
  // Long overviews must be reachable and scrollable without a pointer.
  const shortSnapshot=snapshot;
  snapshot={...snapshot,ci:{state:'pass',jobs:Array.from({length:45},(_,i)=>({name:'Check '+i,state:'pass'}))},humans:{state:'approved',reviewers:Array.from({length:20},(_,i)=>({login:'reviewer'+i,state:'approved'}))}};
  await page.reload();await status().waitFor();await page.waitForFunction(()=>document.querySelector('button[aria-label*="Human approved · CI passing"]'));
  await status().focus();await overview().waitFor();await page.keyboard.press('Tab');
  const ciSummary=overview().getByRole('region',{name:'CI checks'});
  assert.match(await ciSummary.innerText(),/All checks passing/);
  assert.equal(await ciSummary.getByRole('listitem').count(),0,'CI stays a single summary even with many jobs');
  assert.equal(await ciSummary.getByText(/^Check \d+$/).count(),0);
  assert.equal(await overview().evaluate(el=>el===document.activeElement),true,'Tab enters the scrollable overview');
  assert.equal(await overview().evaluate(el=>el.scrollHeight>el.clientHeight),true);
  await page.keyboard.press('PageDown');await page.waitForFunction(()=>document.querySelector('[role="dialog"]')?.scrollTop>0);
  await page.keyboard.press('Escape');await overview().waitFor({state:'hidden'});
  await page.waitForFunction(()=>document.activeElement?.matches('button[aria-label*="Show PR status overview"]'));
  await page.keyboard.press('Enter');await overview().waitFor();
  await page.waitForFunction(()=>document.activeElement?.getAttribute('role')==='dialog');
  await page.keyboard.press('Escape');await overview().waitFor({state:'hidden'});
  snapshot=shortSnapshot;
  // Remount the board after checking outside-click dismissal.
  await page.goto(base+'/projects/1');await status().waitFor();
  const box=await status().boundingBox();await page.mouse.move(box.x+box.width/2,box.y+box.height/2);await page.mouse.down();await page.mouse.move(box.x+box.width/2+35,box.y+box.height/2+30,{steps:5});
  assert.equal(await page.getByText(task.title,{exact:true}).count(),1,'Status mouse gestures do not drag the task');await page.mouse.up();
  const cdp=await page.context().newCDPSession(page);await cdp.send('Input.dispatchTouchEvent',{type:'touchStart',touchPoints:[{x:box.x+box.width/2,y:box.y+box.height/2}]});await page.waitForTimeout(350);assert.equal(await page.getByText(task.title,{exact:true}).count(),1,'Status holds do not start touch dragging');await cdp.send('Input.dispatchTouchEvent',{type:'touchCancel',touchPoints:[]});await cdp.detach();
  await page.keyboard.press('Escape');
  for(const [bots,mark] of [['clean',null],['running','.lucide-clock'],['unknown','.lucide-circle-help'],['changes','.lucide-triangle-alert']]){
   snapshot={...snapshot,bots:{state:bots,reviewers:[{login:'reviewbot',state:bots}]}};await page.reload();await status().waitFor();await page.waitForFunction(b=>document.querySelector('button[aria-label*="Human approved · CI passing"]')?.getAttribute('aria-label').includes(b),{clean:'Completed · no new findings',running:'Review running',unknown:'Unknown',changes:'Changes requested'}[bots]);
   assert.match(await status().locator('svg').first().getAttribute('class'),/text-green-400/);
   if(mark&&bots!=='unknown')assert.equal(await status().locator(mark).count(),1);
   if(!mark)assert.equal(await status().locator('svg').count(),1,'Clean bots remove the secondary badge');
  }
  snapshot={...snapshot,stale:true};await page.reload();await status().waitFor();await page.waitForFunction(()=>document.querySelector('button[aria-label*="PR status is stale"]'));assert.match(await status().locator('svg').first().getAttribute('class'),/text-zinc-400/);
  snapshot={...snapshot,stale:false,bots:{state:'comments',reviewers:[{login:'reviewbot',state:'comments'}]}};await page.reload();await page.getByText(task.title,{exact:true}).click();await page.waitForURL('**/projects/1/tasks/1');await page.getByText('task/pr-links',{exact:true}).first().waitFor();await status().hover();await overview().waitFor();
  assert.match(await overview().innerText(),/Human approved · CI passing/);await page.keyboard.press('Escape');
  await page.setViewportSize({width:390,height:844});await status().tap();await overview().waitFor();const rect=await overview().boundingBox();assert.ok(rect.x>=0&&rect.x+rect.width<=390&&rect.y>=0&&rect.y+rect.height<=844,'Overview fits mobile viewport');
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false);
  if(process.env.KAMACU_BROWSER_SCREENSHOTS)await page.screenshot({path:process.env.KAMACU_BROWSER_SCREENSHOTS+'-mobile.png'});
  await page.keyboard.press('Escape');assert.deepEqual(errors,[]);
  console.log('PASS: shared polling, unified CI/human/bot overview, keyboard focus/scroll/restore, hover/focus/Escape, pin/toggle/outside click, mouse/touch drag isolation, bot badges, stale handling, task page and mobile bounds.');
 }finally{await browser.close()}
})().catch(e=>{console.error(e);process.exit(1)});
