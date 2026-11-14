// Run against Vite preview. All API reads/writes below are isolated fixtures.
const { chromium } = require(process.env.KAMACU_PLAYWRIGHT_PACKAGE || 'playwright');
const assert = require('node:assert/strict');
(async () => {
 const browser = await chromium.launch({ headless:true, executablePath:process.env.KAMACU_BROWSER_EXECUTABLE, args:['--no-sandbox'] });
 try {
  const page = await browser.newPage({viewport:{width:1440,height:1000},hasTouch:true});
  const errors=[]; page.on('pageerror',error=>{errors.push(error.message);console.error('Browser error:',error.message)});
  const base={task_id:1,project_id:1,task_title:'Improve task context',project_name:'Kamacu',session_id:'s',abstract:'The fix is ready. Review the release constraints before continuing.',summary:'Detailed summary',context:'Context for https://github.com/acme/repo/pull/7',question:'Which release should contain this fix?',options:[{id:'yes',label:'Keep the release on hold'}],status:'pending',reason:'',answer:'',revision:1,created_at:3000,updated_at:3000,kind:'question',origin:'observer',delivery_mode:'text'};
  let messages=[{...base,id:'first'},{...base,id:'second',kind:'completion',question:'Work completed',options:[{id:'move_to_review',label:'Move to review and close session'}]}];
  let mode='accept',writes=[],queueReads=0,releaseAnswer,holdCompletions=false,releaseCompletions;
  const prReads=[];
  const project={id:1,name:'Kamacu',github_repo:'acme/repo',workspace_id:1,icon_letters:'KA',icon_color:'blue'};
  await page.route('**/api/**',async route=>{
   const url=new URL(route.request().url()),path=url.pathname;let data=[];
   if(path==='/api/inbox/catch-up'){queueReads++;data=messages.filter(m=>m.status==='pending').map(m=>({...m}));}
   else if(path==='/api/inbox') data={items:url.searchParams.get('view')==='archived'?messages.filter(m=>m.status!=='pending'):messages.filter(m=>m.status==='pending').slice(0,50),pending_count:messages.filter(m=>m.status==='pending').length,has_more:messages.length>50};
   else if(/\/api\/inbox\/[^/]+\/history$/.test(path)) data={items:[{...base,id:'prior-new',created_at:2000,abstract:'Newer prior message',status:'answered'},{...base,id:'prior-old',created_at:1000,abstract:'Older prior message',status:'answered'}],has_more:false};
   else if(/\/api\/inbox\/[^/]+\/(answer|dismiss)$/.test(path)){
    const q=messages.find(m=>m.id===path.split('/')[3]),body=route.request().postDataJSON();writes.push({id:q.id,path,body});
    if(mode==='fail')return route.fulfill({status:500,json:{error:'Could not save answer'}});
    if(mode==='delayed'){const saved={...q,status:'answered'};q.revision++;q.question='A newer question arrived';await new Promise(resolve=>{releaseAnswer=resolve});data={outcome:'accepted',question:saved};}
    else if(mode==='stale'){q.revision++;data={outcome:'stale_revision',question:{...q}};}else{q.status=path.endsWith('/dismiss')?'dismissed':'answered';q.answer=body.answer||body.option_id||'';q.reason=body.option_id||'';data={outcome:path.endsWith('/dismiss')?'dismissed':'accepted',question:{...q}};}
   }
   else if(path==='/api/inbox/tasks/1/completions'){data=messages.filter(m=>m.task_id===1&&m.kind==='completion'&&m.status==='pending').map(m=>({...m}));if(holdCompletions)await new Promise(resolve=>{releaseCompletions=resolve});}
   else if(messages.some(m=>path==='/api/inbox/'+m.id))data={...messages.find(m=>path==='/api/inbox/'+m.id)};
   else if(path==='/api/tasks/1')data={id:1,project_id:1,title:base.task_title,description:'Task **brief**',status:'in_review',branch:'task/context',pr_links:[{repository:project.github_repo,number:7,url:'https://github.com/'+project.github_repo+'/pull/7',role:'implementation'}]};
   else if(/^\/api\/projects\/1\/pull-requests\/(7|8)$/.test(path)){const number=+path.split('/').at(-1);prReads.push({repo:project.github_repo,number,kind:'detail'});data={number,url:'https://github.com/'+project.github_repo+'/pull/'+number,title:project.github_repo==='acme/repo'?(number===8?'Second PR context':'Improve the task context'):'Other repository PR',body:number===8?'Details for PR 8.':project.github_repo==='acme/repo'?'## PR description\n\nFull description from GitHub.':'Description from the other repository.',author:'author',state:'OPEN',commits:3,headRefName:'task/context',baseRefName:'main'};}
   else if(/^\/api\/projects\/1\/pull-requests\/(7|8)\/status$/.test(path)){const number=+path.split('/').at(-2);prReads.push({repo:project.github_repo,number,kind:'status'});data={repository:project.github_repo,number,lifecycle:'open',head:'abcdef0123',ci:{state:'pass',jobs:[{name:'Unit tests',state:'pass',detail:'success'}]},humans:{state:'waiting',reviewers:[{login:project.github_repo==='acme/repo'?'reviewer':'other-reviewer',state:'waiting',detail:'Review requested'}]},bots:{state:'clean',reviewers:[{login:'bot',state:'clean',detail:'Completed on the current commit · no new findings'}]},fetched_at:'2026-11-14T09:00:00Z',stale:false};}
   else if(path === '/api/notifications/rules') data=[{id:'rules',revision:1,status:'active',rules:[]}];
   else if(path==='/api/projects')data=[project];
   else if(path==='/api/projects/1')data=project;
   else if(path==='/api/workspaces')data=[{id:1,name:'Work'}];
   else if(path==='/api/notifications/status')data={pending_count:0,processing:null};
   else if(path==='/api/settings')data=Object.fromEntries(['worktree_base','shell','branch_template','done_session_ttl','github_integration','pr_review_seed'].map(k=>[k,{value:'',default:'',source:'default'}]));
   else if(path==='/api/monitor')data={profiles:[],workspaces:[],usage:{}};
   else if(path==='/api/worktrees')data={counts:{total:0,orphaned:0},projects:[]};
   await route.fulfill({json:data});
  });
  await page.goto(process.argv[2]+'/settings');
  const sidebar=page.getByRole('complementary',{name:'Inbox sidebar'});
  await sidebar.getByRole('button',{name:'Expand inbox sidebar'}).click();
  await sidebar.getByRole('heading',{name:base.question,exact:true}).click();
  assert.equal(await page.getByRole('dialog').count(),0,'message title is plain text');
  const underlying=await page.locator('main').first().elementHandle();
  await underlying.evaluate(el=>{const draft=document.createElement('textarea');draft.id='preserved-draft';draft.value='Unsent current-page draft';el.append(draft)});
  const settingsScroll=page.locator('main .overflow-y-auto').first();
  await settingsScroll.evaluate(el=>el.scrollTop=120);
  const originalScroll=await settingsScroll.evaluate(el=>el.scrollTop);
  const launch=sidebar.getByRole('button',{name:'Catch up',exact:true});
  await launch.click();
  const view=page.getByRole('dialog',{name:'Catch up',exact:true});
  await view.getByRole('button',{name:'Keep the release on hold',exact:true}).waitFor();
  const bounds=await view.boundingBox();assert.deepEqual({x:bounds.x,y:bounds.y,width:bounds.width,height:bounds.height},{x:0,y:0,width:1440,height:1000});
  await view.getByRole('navigation',{name:'Message project and PR references'}).getByText('Kamacu',{exact:true}).waitFor();
  await view.getByRole('navigation',{name:'Message project and PR references'}).getByRole('link',{name:'acme/repo · PR 7',exact:true}).waitFor();
  await view.getByText('Full description from GitHub.',{exact:true}).waitFor();
  await view.getByRole('heading',{name:'Human reviews',exact:true}).waitFor();
  await view.getByRole('heading',{name:'Bot feedback',exact:true}).waitFor();
  await view.locator('summary').filter({hasText:'All 1 checks'}).click();
  await view.getByText('Unit tests',{exact:true}).waitFor();
  const order=await view.locator('section[aria-labelledby="catch-up-history-title"] article').allTextContents();
  assert.match(order[0],/This message/);assert.match(order[1],/Newer prior message/);assert.match(order[2],/Older prior message/);
  await view.getByRole('button',{name:'Keep the release on hold',exact:true}).click();
  await view.locator('summary').filter({hasText:'Write your own answer'}).click();
  await view.getByLabel('Your instructions',{exact:true}).fill('Preserved catch-up draft');
  await page.screenshot({path:'/tmp/catch-up-implemented-desktop.png'});
  messages.push({...base,id:'new-arrival',question:'Arrived after this pass started'});
  await view.getByRole('button',{name:'Skip for now →',exact:true}).click();
  await view.getByRole('heading',{name:'What should happen next?',exact:true}).waitFor();
  assert.equal(writes.length,0,'Skip must not write or reorder the inbox');
  await view.getByRole('button',{name:'Previous',exact:true}).click();
  assert.equal(await view.getByLabel('Your instructions',{exact:true}).inputValue(),'Preserved catch-up draft');
  assert.equal(await view.getByRole('button',{name:'Keep the release on hold',exact:true}).getAttribute('aria-pressed'),'true');
  mode='fail';await view.getByRole('button',{name:'Send reply & next',exact:true}).click();
  await view.getByRole('alert').filter({hasText:'Could not save answer'}).waitFor();
  assert.equal(await view.getByLabel('Your instructions',{exact:true}).inputValue(),'Preserved catch-up draft');
  mode='stale';await view.getByRole('button',{name:'Send reply & next',exact:true}).click();
  await view.getByRole('button',{name:'I’ve reviewed the updated message',exact:true}).waitFor();
  assert.equal(await view.getByRole('button',{name:'Send reply & next',exact:true}).isDisabled(),true);
  await view.getByRole('button',{name:'I’ve reviewed the updated message',exact:true}).click();
  mode='accept';await view.getByRole('button',{name:'Send reply & next',exact:true}).click();
  await view.getByRole('heading',{name:'What should happen next?',exact:true}).waitFor();
  assert.equal(writes.at(-1).body.answer,'Preserved catch-up draft');assert.equal(writes.at(-1).body.revision,2);assert.equal(writes.at(-1).body.option_id,undefined);
  await view.getByRole('button',{name:'Move to review and close session',exact:true}).click();
  await view.getByRole('button',{name:'Confirm & next',exact:true}).click();
  await view.getByRole('heading',{name:'You’ve reached the end of this pass',exact:true}).waitFor();
  assert.equal(writes.at(-1).body.option_id,'move_to_review');assert.equal(queueReads,1);
  await page.keyboard.press('Escape');await view.waitFor({state:'detached'});
  assert.equal(await underlying.evaluate(el=>el===document.querySelector('main')),true,'underlying page remains mounted');
  assert.equal(await page.locator('#preserved-draft').inputValue(),'Unsent current-page draft');
  assert.equal(await settingsScroll.evaluate(el=>el.scrollTop),originalScroll,'underlying scroll position is retained');
  assert.equal(await launch.evaluate(el=>el===document.activeElement),true,'focus returns to launcher');
  await launch.click();await view.getByRole('heading',{name:'Arrived after this pass started',exact:true}).waitFor();
  await view.getByRole('button',{name:'Close',exact:true}).click();await view.waitFor({state:'detached'});
  // The compact inbox is itself a sheet; catch-up must restore that exact place.
  await page.setViewportSize({width:1000,height:900});
  const sheet=page.getByRole('dialog',{name:'Inbox',exact:true});await sheet.waitFor();
  await sheet.getByRole('button',{name:'Catch up',exact:true}).click();await view.waitFor();
  await view.getByRole('button',{name:'Close',exact:true}).focus();
  await page.keyboard.press('Escape');await view.waitFor({state:'detached'});await sheet.waitFor();
  assert.equal(await sheet.getByRole('button',{name:'Catch up',exact:true}).evaluate(el=>el===document.activeElement),true);
  await page.keyboard.press('Escape');await sheet.waitFor({state:'detached'});
  // Mobile entry is on the full inbox page. The same view stacks without overflow.
  await page.setViewportSize({width:390,height:844});await page.goto(process.argv[2]+'/inbox');
  await page.locator('main').getByRole('button',{name:'Catch up',exact:true}).click();await view.waitFor();
  await view.getByRole('button',{name:'Keep the release on hold',exact:true}).waitFor();
  await page.screenshot({path:'/tmp/catch-up-implemented-mobile.png'});
  assert.equal(await view.evaluate(el=>el.scrollWidth<=el.clientWidth),true,'mobile overflow');
  await page.keyboard.press('Escape');await view.waitFor({state:'detached'});
  messages=Array.from({length:51},(_,i)=>({...base,id:'page-'+i,question:'Queue message '+i}));
  await page.locator('main').getByRole('button',{name:'Catch up',exact:true}).click();
  for(let i=0;i<51;i++){await view.getByRole('heading',{name:'Queue message '+i,exact:true}).waitFor();await view.getByRole('button',{name:'Skip for now →',exact:true}).click()}
  await view.getByRole('heading',{name:'You’ve reached the end of this pass',exact:true}).waitFor();
  assert.equal(messages.filter(m=>m.status==='pending').length,51);
  await page.keyboard.press('Escape');messages=[];
  await page.locator('main').getByRole('button',{name:'Catch up',exact:true}).click();
  await view.getByRole('heading',{name:'You’re all caught up',exact:true}).waitFor();
  await page.keyboard.press('Escape');
  // Native approvals must remain terminal interactions, including Scratchpad.
  messages=[{...base,id:'native',task_id:0,project_id:0,project_name:'',task_title:'Scratchpad',delivery_mode:'terminal',context:'Native terminal approval',question:'Approve in the terminal?'}];
  await page.locator('main').getByRole('button',{name:'Catch up',exact:true}).click();
  await view.getByRole('link',{name:/^Open Scratchpad/}).waitFor();
  assert.equal(await view.getByRole('button',{name:'Keep the release on hold',exact:true}).count(),0);
  assert.equal(await view.locator('textarea').count(),0);
  await page.keyboard.press('Escape');
  messages=[{...base,id:'claude-choice',delivery_mode:'claude_question'}];
  await page.locator('main').getByRole('button',{name:'Catch up',exact:true}).click();
  await view.getByRole('button',{name:'Keep the release on hold',exact:true}).waitFor();
  assert.equal(await view.locator('textarea').count(),0,'Claude menus cannot accept arbitrary text');
  await page.keyboard.press('Escape');
  messages=[{...base,id:'notification',task_id:0,project_id:0,task_title:'',project_name:'',origin:'notification',source_url:'https://github.com/acme/repo/pull/7',linked_tasks:[{task_id:1,project_id:1,title:base.task_title,status:'in_review',role:'implementation'}],options:[{id:'continue',label:'Continue the linked task',action_summary:'Send saved instructions',action:{version:1,actions:[{id:'continue',kind:'continue_task',project_id:1,task_id:1,instructions:'Saved task instructions'}]}}]}];
  await page.locator('main').getByRole('button',{name:'Catch up',exact:true}).click();
  await view.getByRole('button',{name:'Continue the linked task',exact:true}).click();
  await view.getByRole('button',{name:'Confirm & next',exact:true}).click();
  await view.getByRole('heading',{name:'You’ve reached the end of this pass',exact:true}).waitFor();
  assert.equal(writes.at(-1).body.option_id,'continue');
  assert.equal(writes.at(-1).body.instructions,undefined,'only the saved action identity is submitted');
  await page.keyboard.press('Escape');
  messages=[{...base,id:'dismiss-first'},{...base,id:'dismiss-second',question:'Message after dismissal'}];
  await page.locator('main').getByRole('button',{name:'Catch up',exact:true}).click();
  await view.getByRole('button',{name:'Dismiss message',exact:true}).click();
  await view.getByRole('heading',{name:'Message after dismissal',exact:true}).waitFor();
  assert.equal(messages[0].status,'dismissed');
  assert.equal(writes.at(-1).path,'/api/inbox/dismiss-first/dismiss');
  await page.keyboard.press('Escape');
  // A mutation response must not overwrite a later revision already polled.
  messages=[{...base,id:'delayed-first'},{...base,id:'delayed-second',question:'Message after delayed response'}];
  mode='delayed';
  await page.locator('main').getByRole('button',{name:'Catch up',exact:true}).click();
  await view.getByRole('button',{name:'Keep the release on hold',exact:true}).click();
  await view.getByRole('button',{name:'Confirm & next',exact:true}).click();
  await view.getByRole('heading',{name:'A newer question arrived',exact:true}).waitFor();
  assert.equal(typeof releaseAnswer,'function');releaseAnswer();
  await view.getByRole('status').filter({hasText:'changed again while your response was being saved'}).waitFor();
  await view.getByRole('heading',{name:'A newer question arrived',exact:true}).waitFor();
  await view.getByRole('button',{name:'I’ve reviewed the updated message',exact:true}).waitFor();
  await page.keyboard.press('Escape');mode='accept';
  // A grouped notification loads only the expanded PR, not every referenced PR.
  messages=[{...base,id:'many-prs',context:base.context+' https://github.com/acme/repo/pull/8'}];
  await page.locator('main').getByRole('button',{name:'Catch up',exact:true}).click();
  const prs=view.getByRole('region',{name:'Related pull requests'});
  const extraPR=prs.locator('details').filter({has:page.getByRole('link',{name:'acme/repo · PR 8',exact:true})});
  await extraPR.locator('summary').first().waitFor();
  assert.equal(prReads.some(r=>r.number===8),false,'collapsed PR makes no detail/status request');
  const reference=view.getByRole('navigation',{name:'Message project and PR references'}).getByRole('link',{name:'acme/repo · PR 8',exact:true});
  await reference.hover();
  const preview=page.getByRole('dialog',{name:'acme/repo PR 8 status overview',exact:true});
  await preview.getByRole('heading',{name:'Second PR context',exact:true}).waitFor();
  const linkBounds=await reference.boundingBox(),previewBounds=await preview.boundingBox();
  assert(previewBounds.y>=linkBounds.y+linkBounds.height || previewBounds.y+previewBounds.height<=linkBounds.y,'preview is anchored clear of its reference link');
  await preview.getByText('All checks passing',{exact:true}).waitFor();
  await preview.getByRole('heading',{name:'Human reviews',exact:true}).waitFor();
  await preview.getByText('@reviewer',{exact:true}).hover();
  await page.waitForTimeout(220);
  assert.equal(await preview.isVisible(),true,'preview stays open while reading it');
  await page.keyboard.press('Escape');await preview.waitFor({state:'detached'});
  await view.waitFor();
  await reference.focus();
  await preview.getByRole('heading',{name:'Second PR context',exact:true}).waitFor();
  await page.keyboard.press('Escape');await preview.waitFor({state:'detached'});
  assert.equal(await reference.evaluate(el=>el===document.activeElement),true,'preview preserves link focus');
  const previewButton=reference.locator('..').getByRole('button',{name:/Show PR status overview/});
  await previewButton.tap();
  await page.mouse.move(1430,20);
  await preview.getByRole('heading',{name:'Second PR context',exact:true}).waitFor();
  await page.keyboard.press('Escape');await preview.waitFor({state:'detached'});
  await view.waitFor();
  // Holding a touch on a native link is not a hover and must not open a preview.
  await page.mouse.move(1430,20);
  const referenceBox=await reference.boundingBox();
  const touch=await page.context().newCDPSession(page);
  await touch.send('Input.dispatchTouchEvent',{type:'touchStart',touchPoints:[{x:referenceBox.x+referenceBox.width/2,y:referenceBox.y+referenceBox.height/2}]});
  await page.waitForTimeout(220);
  const previewDuringTouch=await preview.count();
  await touch.send('Input.dispatchTouchEvent',{type:'touchCancel',touchPoints:[]});await touch.detach();
  assert.equal(previewDuringTouch,0,'touch-and-hold on the link does not trigger a hover preview');
  await page.context().route('https://github.com/**',route=>route.fulfill({body:'Mock GitHub PR destination'}));
  const openedLink=page.waitForEvent('popup');await reference.click();
  const github=await openedLink;await github.waitForLoadState();
  assert.equal(github.url(),'https://github.com/acme/repo/pull/8');await github.close();
  // A link must also activate after the user has read its already-open preview.
  await page.mouse.move(1430,20);await reference.hover();
  await preview.getByRole('heading',{name:'Second PR context',exact:true}).waitFor();
  const hoveredPopup=page.waitForEvent('popup');await reference.click();
  const hoveredGithub=await hoveredPopup;await hoveredGithub.waitForLoadState();
  assert.equal(hoveredGithub.url(),'https://github.com/acme/repo/pull/8');await hoveredGithub.close();
  // Keyboard activation still follows the link rather than pinning the preview.
  await reference.focus();
  const keyboardPopup=page.waitForEvent('popup');await page.keyboard.press('Enter');
  const keyboardGithub=await keyboardPopup;await keyboardGithub.waitForLoadState();
  assert.equal(keyboardGithub.url(),'https://github.com/acme/repo/pull/8');await keyboardGithub.close();
  // The first click on another control must both dismiss the preview and act.
  await page.mouse.move(1430,20);await reference.hover();
  await preview.getByRole('heading',{name:'Second PR context',exact:true}).waitFor();
  const responseChoice=view.getByRole('button',{name:'Keep the release on hold',exact:true});
  await responseChoice.click();await preview.waitFor({state:'detached'});
  assert.equal(await responseChoice.getAttribute('aria-pressed'),'true','an outside click reaches the intended control');
  const previewReads=prReads.filter(r=>r.number===8).length;
  await extraPR.locator('summary').first().click();
  await extraPR.getByText('Details for PR 8.',{exact:true}).waitFor();
  assert.equal(prReads.filter(r=>r.number===8).length,previewReads,'expanded PR reuses the preview cache');
  assert(prReads.some(r=>r.number===8&&r.kind==='status'));
  await page.keyboard.press('Escape');
  // The same project and PR number in a different repository is a new cache identity.
  project.github_repo='acme/other';messages=[{...base,id:'other-repo',context:'https://github.com/acme/other/pull/7'}];
  await page.locator('main').getByRole('button',{name:'Catch up',exact:true}).click();
  const otherPR=view.getByRole('region',{name:'Related pull requests'}).locator('details').filter({has:page.getByRole('link',{name:'acme/other · PR 7',exact:true})});
  await otherPR.locator('summary').first().waitFor();
  if(await otherPR.getAttribute('open')===null)await otherPR.locator('summary').first().click();
  await otherPR.getByText('Description from the other repository.',{exact:true}).waitFor();
  await otherPR.getByText('@other-reviewer',{exact:true}).waitFor();
  assert.equal(await otherPR.getByText('@reviewer',{exact:true}).count(),0);
  await page.keyboard.press('Escape');
  // A delayed task-page completion scan must not archive reports under Catch up.
  await page.setViewportSize({width:1440,height:1000});
  messages=[{...base,id:'covered-completion',kind:'completion',question:'Work completed',options:[{id:'exit',label:'Close session'}]}];
  holdCompletions=true;
  const scan=page.waitForRequest(r=>new URL(r.url()).pathname==='/api/inbox/tasks/1/completions');
  await page.goto(process.argv[2]+'/projects/1/tasks/1');await scan;
  await sidebar.getByRole('button',{name:'Catch up',exact:true}).click();
  await view.getByRole('heading',{name:'What should happen next?',exact:true}).waitFor();
  const beforeRelease=writes.length;holdCompletions=false;releaseCompletions();
  await page.waitForTimeout(250);
  assert.equal(writes.length,beforeRelease,'covered task cannot dismiss a delayed completion snapshot');
  assert.equal(messages[0].status,'pending');
  const resumedDismiss=page.waitForResponse(r=>new URL(r.url()).pathname==='/api/inbox/covered-completion/dismiss');
  await view.getByRole('button',{name:'Close',exact:true}).click();await view.waitFor({state:'detached'});
  await resumedDismiss;
  assert.equal(messages[0].status,'dismissed','visible task starts a fresh completion visit');
  assert.deepEqual(errors,[]);
  console.log('Catch-up review regressions, fullscreen, references, newest-first history, full PR status, stable 51-message pass, non-mutating skip, draft/revision/error handling, completion actions, new arrivals, preserved page/focus, compact sheet and mobile passed.');
 } finally {await browser.close()}
})().catch(error=>{console.error(error);process.exit(1)});
