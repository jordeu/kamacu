// Run against a built UI served by Vite preview. API responses are isolated fixtures.
const { chromium } = require(process.env.KAMACU_PLAYWRIGHT_PACKAGE || 'playwright');
const assert = require('node:assert/strict');
(async () => {
 const browser = await chromium.launch({headless:true, executablePath:process.env.KAMACU_BROWSER_EXECUTABLE,args:['--no-sandbox']});
 try {
  const page = await browser.newPage({viewport:{width:1440,height:1000}});
  const errors = []; page.on('pageerror', e => errors.push(e.message));
  let pending = true, answer, paginated = false, completion = false, dismissMode = "accept", multiple = false;
  let releaseArchive;
  let releaseNextPage;
  let holdNextPage = false;
  const q = {id:'sidebar-question',task_id:1,project_id:1,task_title:'Improve navigation',project_name:'Kamacu',session_id:'s',abstract:'Task links currently preserve the selected tab, adding a click before replying. I recommend focusing the originating agent terminal only when opening a task from an inbox message; ordinary task navigation would keep the current tab. Choose whether inbox task links should focus the terminal.',summary:'Detailed task summary',context:'Decision context',question:'Focus terminal when opening a task?',options:[{id:'yes',label:'Focus terminal'}],status:'pending',revision:1,created_at:Date.now(),updated_at:Date.now(),kind:'question'};
  await page.route('**/api/**', async route => {
   q.kind = completion ? 'completion' : 'question';
   q.options = completion ? [{id:'seen',label:'Seen'},{id:'exit',label:'Seen and exit session'}] : [{id:'yes',label:'Focus terminal'}];
   const path = new URL(route.request().url()).pathname;
   let data = [];
   if(path === '/api/inbox') {
    const params = new URL(route.request().url()).searchParams;
    if (holdNextPage && params.get('offset') === '50') {
     await new Promise(resolve => { releaseNextPage = resolve; });
    }
    const firstPage = paginated && params.get('offset') !== '50';
    data={items:params.get('view') === 'archived' || !pending ? [] : firstPage ? [{...q,id:'first-page',abstract:'Another message'}] : multiple ? [q,{...q,id:"second-question",question:"Another question"}] : [q],pending_count:pending?paginated?51:1:0,has_more:firstPage};
   }
   else if(path === '/api/inbox/'+q.id) data={...q,status:pending?'pending':'answered'};
   else if(path.endsWith('/dismiss') && dismissMode === 'error') return route.fulfill({status:500,json:{error:'Could not archive'}});
   else if(path.endsWith('/dismiss') && dismissMode === 'stale') data={outcome:'stale_revision',question:{...q,revision:2}};
   else if(path.endsWith('/dismiss')) { answer=route.request().postDataJSON(); pending=false; if (dismissMode === 'hold') await new Promise(resolve => { releaseArchive = resolve; }); data={outcome:'accepted',question:{...q,status:'dismissed'}}; }
   else if(path.endsWith('/answer')) { answer=route.request().postDataJSON(); pending=false; data={outcome:'accepted',question:{...q,status:'answered',reason:answer.option_id === 'exit' ? 'exit' : '',answer:answer.option_id?'Focus terminal':answer.answer}}; }
   else if(path === '/api/tasks/1') data={id:1,project_id:1,title:'Improve navigation',description:'',status:'todo',position:0};
   else if(path === '/api/projects/1') data={id:1,name:'Kamacu',workspace_id:1};
   else if(path === '/api/workspaces') data=[{id:1,name:'Work'}];
   else if(path === '/api/projects') data=[{id:1,name:'Kamacu',workspace_id:1,icon_letters:'KA',icon_color:'blue'}];
   else if(path === '/api/worktrees') data={counts:{total:0,orphaned:0},projects:[]};
   else if(path === '/api/settings') data=Object.fromEntries(['worktree_base','shell','branch_template','done_session_ttl','github_integration','pr_review_seed'].map(k=>[k,{value:'',default:'',source:'default'}]));
   await route.fulfill({json:data});
  });
  await page.goto(process.argv[2]+'/settings');
  const sidebar = page.getByRole('complementary',{name:'Inbox sidebar'});
  await sidebar.getByLabel('1 pending messages',{exact:true}).waitFor();
  assert.match(await sidebar.getByRole('button',{name:'Expand inbox sidebar'}).getAttribute('class'), /ring-orange/);
  assert.match(await page.locator('link[rel=icon]').getAttribute('href'), /^data:image\/svg\+xml,/);
  assert.match(decodeURIComponent(await page.locator('link[rel=icon]').getAttribute('href')), />1<\/text>/);
  await sidebar.getByRole('button',{name:'Expand inbox sidebar'}).click();
  await sidebar.getByText(q.abstract,{exact:true}).waitFor();
  // Message titles open the shared full view without navigating or remounting the current page.
  const underlyingInput = page.locator('main input').first();
  const hasInput = await underlyingInput.count();
  if (hasInput) await underlyingInput.fill('Preserved draft');
  const mainHandle = await page.locator('main').elementHandle();
  const title = sidebar.getByRole('button',{name:q.question,exact:true});
  await title.hover();
  assert.equal(await page.getByRole('dialog').count(),0,'hover must not open a message');
  await title.click();
  const dialog = page.getByRole('dialog',{name:'Inbox message'});
  await dialog.getByText(q.context,{exact:true}).waitFor();
  await dialog.getByLabel('Write your own answer').fill('Draft reply');
  assert.equal(new URL(page.url()).pathname,'/settings');
  await page.keyboard.press('Escape');
  await dialog.waitFor({state:'detached'});
  assert.equal(await mainHandle.evaluate(el => el === document.querySelector('main')),true);
  if (hasInput) assert.equal(await underlyingInput.inputValue(),'Preserved draft');
  await title.click();
  await dialog.getByRole('button',{name:'Close',exact:true}).click();
  await dialog.waitFor({state:'detached'});
  assert.equal(new URL(page.url()).pathname,'/settings');
  multiple=true; await page.reload();
  await sidebar.getByRole('button',{name:'Another question',exact:true}).waitFor();
  await title.click();
  await dialog.getByText(q.context,{exact:true}).waitFor();
  await page.keyboard.press('Escape');
  await dialog.waitFor({state:'detached'});
  assert.equal(await title.evaluate(el => el === document.activeElement),true,'focus must return to the clicked title');
  multiple=false; await page.reload();
  await title.waitFor();
  await page.screenshot({path:'/tmp/inbox-sidebar-desktop.png'});
  await page.setViewportSize({width:390,height:844});
  await page.screenshot({path:'/tmp/inbox-sidebar-question-mobile.png'});
  assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth));
  await page.setViewportSize({width:1440,height:1000});
  await sidebar.getByRole('button',{name:'Focus terminal',exact:true}).click();
  await sidebar.getByRole('status').waitFor();
  assert.equal(new URL(page.url()).pathname,'/settings'); assert.equal(answer.option_id,'yes'); assert.equal(answer.revision,1);
  await page.waitForFunction(() => document.querySelector('link[rel=icon]').getAttribute('href') === '/favicon.svg');
  pending=true; await page.reload();
  await sidebar.getByText(q.abstract,{exact:true}).waitFor();
  assert.equal(await sidebar.locator('textarea').count(),0);
  await sidebar.getByRole('button',{name:'Dismiss',exact:true}).click();
  await sidebar.getByText('Message dismissed and archived.',{exact:true}).waitFor();
  pending=true; completion=true; await page.reload();
  await sidebar.getByText(q.abstract,{exact:true}).waitFor();
  assert.equal(await sidebar.getByRole('button',{name:'Seen',exact:true}).count(),0);
  assert.equal(await sidebar.getByRole('link',{name:/Full context/}).count(),0);
  assert.equal(await sidebar.locator('textarea').count(),0);
  assert.equal(await sidebar.locator('article button').count(),4);
  await sidebar.getByRole('button',{name:q.question,exact:true}).click();
  await dialog.getByText('What was done',{exact:true}).waitFor();
  await dialog.getByRole('button',{name:'OK, exit session',exact:true}).waitFor();
  await dialog.getByRole('button',{name:'Dismiss',exact:true}).waitFor();
  await page.keyboard.press('Escape');
  await page.screenshot({path:'/tmp/inbox-sidebar-completion.png'});
  await sidebar.getByRole('button',{name:'OK, exit session',exact:true}).click();
  await sidebar.getByText('Message archived. Agent session is closing.',{exact:true}).waitFor();
  assert.equal(answer.option_id,'exit');
  // Failed or stale archive attempts must leave the user on the current page.
  pending=true; dismissMode='error'; await page.reload();
  await sidebar.getByRole('button',{name:'Open task',exact:true}).click();
  await sidebar.getByRole('alert').waitFor();
  assert.equal(new URL(page.url()).pathname,'/settings');
  assert.equal(pending,true);
  await sidebar.getByRole('button',{name:'Collapse inbox sidebar'}).waitFor();
  dismissMode='stale';
  await sidebar.getByRole('button',{name:'Open task',exact:true}).click();
  await sidebar.getByText('This question was reopened. Review its current version before answering.',{exact:true}).waitFor();
  assert.equal(new URL(page.url()).pathname,'/settings');
  assert.equal(pending,true);
  await sidebar.getByRole('button',{name:'Collapse inbox sidebar'}).waitFor();
  // Stale results must be announced within the modal, where the user can read them.
  await sidebar.getByRole('button',{name:q.question,exact:true}).click();
  await dialog.getByRole('button',{name:'Dismiss',exact:true}).click();
  await dialog.getByRole('status').filter({hasText:'This question was reopened.'}).waitFor();
  assert.equal(new URL(page.url()).pathname,'/settings');
  await page.keyboard.press('Escape');
  await dialog.waitFor({state:'detached'});
  // Opening the task must archive using dismiss, without sending an answer.
  dismissMode='accept';
  await sidebar.getByRole('button',{name:'Open task',exact:true}).click();
  await page.waitForURL('**/projects/1/tasks/1?feedback_session=s');
  assert.equal(pending,false);
  await sidebar.getByRole('button',{name:'Expand inbox sidebar'}).waitFor();
  assert.equal(await page.evaluate(()=>localStorage.getItem('kamacu:inbox-collapsed')),'1');
  assert.equal(answer.option_id,undefined);
  // Polling can archive a message while its dialog remains open.
  pending=true; await page.goto(process.argv[2]+'/settings');
  await sidebar.getByRole('button',{name:'Expand inbox sidebar'}).click();
  await sidebar.getByRole('button',{name:q.question,exact:true}).click();
  await dialog.getByText('What was done',{exact:true}).waitFor();
  pending=false;
  await dialog.getByText('Answered ·', {exact:false}).waitFor();
  await dialog.getByRole('button',{name:/^Open task/}).click();
  await page.waitForURL('**/projects/1/tasks/1?feedback_session=s');
  await dialog.waitFor({state:'detached'});
  await sidebar.getByRole('button',{name:'Collapse inbox sidebar'}).click();
  completion=false;
  pending=true; await page.goto(process.argv[2]+'/settings');
  await sidebar.getByRole('button',{name:'Expand inbox sidebar'}).click();
  await sidebar.getByRole('link',{name:'Full context / custom reply',exact:true}).click();
  await page.locator('#message-sidebar-question[open]').waitFor();
  await page.getByRole('button',{name:'Archived',exact:true}).click();
  await page.getByText('No archived questions',{exact:true}).waitFor();
  await sidebar.getByRole('link',{name:'Full context / custom reply',exact:true}).click();
  await page.locator('#message-sidebar-question[open]').waitFor();
  paginated=true;
  await page.reload();
  await page.waitForFunction(() => decodeURIComponent(document.querySelector('link[rel=icon]').getAttribute('href')).includes('>51</text>'));
  holdNextPage = true;
  const nextPageRequest = page.waitForRequest(request => new URL(request.url()).searchParams.get('offset') === '50');
  await sidebar.getByRole('button',{name:'Next',exact:true}).click();
  await nextPageRequest;
  await sidebar.getByText('Loading messages…',{exact:true}).waitFor();
  await sidebar.getByLabel('51 pending messages',{exact:true}).waitFor();
  assert.match(decodeURIComponent(await page.locator('link[rel=icon]').getAttribute('href')), />51<\/text>/);
  holdNextPage = false;
  releaseNextPage();
  await sidebar.getByRole('link',{name:'Full context / custom reply',exact:true}).click();
  await page.locator('#message-sidebar-question[open]').waitFor();
  await page.reload();
  await page.locator('#message-sidebar-question[open]').waitFor();
  await page.setViewportSize({width:390,height:844});
  await page.screenshot({path:'/tmp/inbox-sidebar-mobile.png'});
  assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'mobile overflow');
  await sidebar.getByRole('button',{name:'Collapse inbox sidebar'}).click();
  await page.reload(); await sidebar.getByRole('button',{name:'Expand inbox sidebar'}).waitFor();
  // Hold the POST response until polling removes its card. Navigation must survive unmount.
  completion=true; paginated=false;
  for (const surface of ['sidebar', 'page']) {
    pending=true; dismissMode='hold'; releaseArchive=undefined;
    await page.goto(process.argv[2]+(surface === 'sidebar' ? '/settings' : '/inbox'));
    const card = surface === 'sidebar' ? sidebar.locator('article') : page.locator('main details');
    if (surface === 'sidebar') await sidebar.getByRole('button',{name:'Expand inbox sidebar'}).click();
    else await card.locator('summary').click();
    await card.getByRole('button',{name:/^Open task/}).click();
    await card.waitFor({state:'detached'});
    assert.equal(typeof releaseArchive,'function','archive response should still be held');
    assert.equal(new URL(page.url()).pathname,surface === 'sidebar' ? '/settings' : '/inbox');
    releaseArchive();
    await page.waitForURL('**/projects/1/tasks/1?feedback_session=s');
    await sidebar.getByRole('button',{name:'Expand inbox sidebar'}).waitFor();
  }
  assert.deepEqual(errors,[]); console.log('Sidebar options, dismissal, completion actions, archive-on-open, persistence, full-context links, pagination, mobile layout and navigation after polling removes cards in both inbox views passed.');
 } finally {await browser.close();}
})().catch(e=>{console.error(e);process.exit(1)});
