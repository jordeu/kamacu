// Run against Vite preview with isolated API fixtures; no real processing starts.
const { chromium } = require(process.env.KAMACU_PLAYWRIGHT_PACKAGE || 'playwright');
const assert = require('node:assert/strict');
(async () => {
 const browser = await chromium.launch({headless:true, executablePath:process.env.KAMACU_BROWSER_EXECUTABLE,args:['--no-sandbox']});
 try {
  const page = await browser.newPage({viewport:{width:1440,height:1000}});
  const errors = []; page.on('pageerror', error => errors.push(error.message));
  const base = process.argv[2]; assert.ok(base, 'Pass the preview URL');
  let run = null, starts = 0;
  const fetching = () => ({id:'run',status:'running',notification_count:42,action_count:0,created_at:Date.now(),updated_at:Date.now(),error:'',progress:{stage:'fetching',completed:17,total:42,stage_started_at:Date.now(),github_requests:62,model_passes:0,lookups:0}});
  await page.route('**/api/**', route => {
   const path = new URL(route.request().url()).pathname;
   let data = [];
   if (path === '/api/events' || path === '/api/pending') return route.fulfill({contentType:'text/event-stream',body:': fixture\n\n'});
   if (path === '/api/notifications/process') { starts++; run = fetching(); data = run; }
   else if (path === '/api/notifications/status') data = {pending_count:run?.status === 'completed' ? 0 : 42,model_configured:true,processing:run};
   else if (path === '/api/notifications') data = {items:[],pending_count:42,has_more:false};
   else if (path === '/api/inbox') data = {items:[],pending_count:0,has_more:false};
   else if (path === '/api/inbox/count') data = {count:0};
   else if (path === '/api/workspaces') data = [{id:1,name:'Work'}];
   else if (path === '/api/projects') data = [{id:1,name:'Kamacu',workspace_id:1,icon_letters:'KA',icon_color:'blue'}];
   else if (path === '/api/settings') data = {github_integration:{value:'on'}};
   return route.fulfill({json:data});
  });
  await page.goto(base + '/notifications');
  const main = page.locator('main').last();
  const panel = main.getByText('Turn incoming activity into actionable inbox messages.',{exact:true}).locator('..');
  await panel.waitFor();
  const mainBox = await main.boundingBox(), panelBox = await panel.boundingBox();
  assert.ok(panelBox.width >= mainBox.width - 52, 'Processing panel must span the page content');
  const button = main.getByRole('button',{name:'Process pending notifications',exact:true});
  await button.click();
  await main.getByText('Fetching activity from GitHub',{exact:true}).waitFor();
  assert.equal(await main.getByRole('progressbar').getAttribute('aria-valuenow'),'17');
  assert.equal(await main.getByRole('button',{name:'Processing 42 notifications',exact:true}).isDisabled(),true);
  await page.screenshot({path:'/tmp/kamacu-notifications-progress-desktop.png',fullPage:true});
  run.progress = {...run.progress,stage:'analyzing',completed:0,total:0,model_passes:1,stage_started_at:Date.now()};
  await main.getByText('Analyzing notifications',{exact:true}).waitFor();
  assert.equal(await main.getByRole('progressbar').count(),0,'Model work must not show a fabricated percentage');
  await main.getByText('Analysis pass 1',{exact:true}).waitFor();
  run.progress = {...run.progress,stage:'checking_updates',completed:31,total:42};
  await main.getByText('Checking discussions for new activity',{exact:true}).waitFor();
  assert.equal(await main.getByRole('progressbar').getAttribute('aria-valuenow'),'31');
  run = {...run,status:'completed',action_count:3};
  await main.getByText('42 notifications processed · 3 inbox actions created.',{exact:true}).waitFor();
  assert.equal(await main.getByRole('progressbar').count(),0);
  assert.equal(starts,1,'Polling progress must not start another processing run');
  run = fetching();
  await page.setViewportSize({width:390,height:844});
  await page.reload();
  await page.locator('main').last().getByText('Fetching activity from GitHub',{exact:true}).waitFor();
  assert.equal(await page.evaluate(()=>document.documentElement.scrollWidth>innerWidth),false,'Mobile layout overflowed');
  await page.screenshot({path:'/tmp/kamacu-notifications-progress-mobile.png',fullPage:true});
  assert.deepEqual(errors,[]);
  console.log('PASS: full-width panel, process click, live fetch/analysis/update/completion transitions, no duplicate run, mobile layout.');
 } finally { await browser.close(); }
})().catch(error=>{console.error(error);process.exit(1)});
