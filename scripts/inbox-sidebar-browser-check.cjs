// Run against a built UI served by Vite preview. API responses are isolated fixtures.
const { chromium } = require(process.env.KAMACU_PLAYWRIGHT_PACKAGE || 'playwright');
const assert = require('node:assert/strict');
(async () => {
 const browser = await chromium.launch({headless:true, executablePath:process.env.KAMACU_BROWSER_EXECUTABLE,args:['--no-sandbox']});
 try {
  const page = await browser.newPage({viewport:{width:1440,height:1000}});
  const errors = []; page.on('pageerror', e => errors.push(e.message));
  let pending = true, answer, paginated = false;
  const q = {id:'sidebar-question',task_id:1,project_id:1,task_title:'Improve navigation',project_name:'Kamacu',session_id:'s',abstract:'Should task links focus the agent terminal?',summary:'Detailed task summary',context:'Decision context',question:'Focus terminal when opening a task?',options:[{id:'yes',label:'Focus terminal'}],status:'pending',revision:1,created_at:Date.now(),updated_at:Date.now(),kind:'question'};
  await page.route('**/api/**', async route => {
   const path = new URL(route.request().url()).pathname;
   let data = [];
   if(path === '/api/inbox') {
    const params = new URL(route.request().url()).searchParams;
    const firstPage = paginated && params.get('offset') !== '50';
    data={items:params.get('view') === 'archived' || !pending ? [] : firstPage ? [{...q,id:'first-page',abstract:'Another message'}] : [q],pending_count:pending?paginated?51:1:0,has_more:firstPage};
   }
   else if(path === '/api/inbox/'+q.id) data={...q,status:pending?'pending':'answered'};
   else if(path.endsWith('/answer')) { answer=route.request().postDataJSON(); pending=false; data={outcome:'accepted',question:{...q,status:'answered',answer:answer.option_id?'Focus terminal':answer.answer}}; }
   else if(path === '/api/workspaces') data=[{id:1,name:'Work'}];
   else if(path === '/api/projects') data=[{id:1,name:'Kamacu',workspace_id:1,icon_letters:'KA',icon_color:'blue'}];
   else if(path === '/api/worktrees') data={counts:{total:0,orphaned:0},projects:[]};
   else if(path === '/api/settings') data=Object.fromEntries(['worktree_base','shell','branch_template','done_session_ttl','github_integration','pr_review_seed'].map(k=>[k,{value:'',default:'',source:'default'}]));
   await route.fulfill({json:data});
  });
  await page.goto(process.argv[2]+'/settings');
  const sidebar = page.getByRole('complementary',{name:'Inbox sidebar'});
  await sidebar.getByLabel('1 pending messages',{exact:true}).waitFor();
  await sidebar.getByRole('button',{name:'Expand inbox sidebar'}).click();
  await sidebar.getByText(q.abstract,{exact:true}).waitFor();
  await page.screenshot({path:'/tmp/inbox-sidebar-desktop.png'});
  await sidebar.getByRole('button',{name:'Focus terminal',exact:true}).click();
  await sidebar.getByRole('status').waitFor();
  assert.equal(new URL(page.url()).pathname,'/settings'); assert.equal(answer.option_id,'yes'); assert.equal(answer.revision,1);
  pending=true; await page.reload();
  await sidebar.getByText(q.abstract,{exact:true}).waitFor();
  await sidebar.getByLabel('Write a reply').fill('Please keep the current tab');
  await sidebar.getByRole('button',{name:'Collapse inbox sidebar'}).click();
  await sidebar.getByRole('button',{name:'Expand inbox sidebar'}).click();
  assert.equal(await sidebar.getByLabel('Write a reply').inputValue(),'Please keep the current tab','collapse discarded draft');
  await sidebar.getByRole('button',{name:'Send reply',exact:true}).click();
  await sidebar.getByRole('status').waitFor(); assert.equal(answer.answer,'Please keep the current tab'); assert.equal(new URL(page.url()).pathname,'/settings');
  pending=true; await page.reload();
  await sidebar.getByRole('link',{name:'Open full message',exact:true}).click();
  await page.locator('#message-sidebar-question[open]').waitFor();
  await page.getByRole('button',{name:'Archived',exact:true}).click();
  await page.getByText('No archived questions',{exact:true}).waitFor();
  await sidebar.getByRole('link',{name:'Open full message',exact:true}).click();
  await page.locator('#message-sidebar-question[open]').waitFor();
  paginated=true;
  await page.reload();
  await sidebar.getByRole('button',{name:'Next',exact:true}).click();
  await sidebar.getByRole('link',{name:'Open full message',exact:true}).click();
  await page.locator('#message-sidebar-question[open]').waitFor();
  await page.reload();
  await page.locator('#message-sidebar-question[open]').waitFor();
  await page.setViewportSize({width:390,height:844});
  await page.screenshot({path:'/tmp/inbox-sidebar-mobile.png'});
  assert.ok(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth),'mobile overflow');
  await sidebar.getByRole('button',{name:'Collapse inbox sidebar'}).click();
  await page.reload(); await sidebar.getByRole('button',{name:'Expand inbox sidebar'}).waitFor();
  assert.deepEqual(errors,[]); console.log('Sidebar option reply, text reply, route preservation, draft preservation, persistence, archived-view and paginated full-message links, direct reload and mobile checks passed.');
 } finally {await browser.close();}
})().catch(e=>{console.error(e);process.exit(1)});
