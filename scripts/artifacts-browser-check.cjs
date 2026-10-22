// Run via KAMACU_BROWSER_TEST=1 go test ./internal/api -run '^TestArtifactsBrowser$'.
// KAMACU_PLAYWRIGHT_PACKAGE names an installed Playwright package; optionally
// set KAMACU_BROWSER_EXECUTABLE and KAMACU_ARTIFACT_SCREENSHOTS (directory).
const { chromium } = require(process.env.KAMACU_PLAYWRIGHT_PACKAGE || 'playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const os = require('node:os');
const path = require('node:path');
const http = require('node:http');
const dgram = require('node:dgram');
const { execFileSync } = require('node:child_process');

(async () => {
  const base = process.argv[2];
  const externalHits = [];
  const external = http.createServer((req, res) => { externalHits.push(req.url); res.setHeader('Content-Type', 'text/html'); res.end('<h1>External navigation</h1>'); });
  await new Promise(resolve => external.listen(0, '127.0.0.1', resolve));
  const outside = `http://127.0.0.1:${external.address().port}`;
  const stun = dgram.createSocket('udp4');
  let stunPackets = 0;
  stun.on('message', () => stunPackets++);
  await new Promise(resolve => stun.bind(0, '127.0.0.1', resolve));
  const browser = await chromium.launch({ headless: true,
    executablePath: process.env.KAMACU_BROWSER_EXECUTABLE,
    args: ['--no-sandbox', ...(process.env.KAMACU_BROWSER_ARGS ? JSON.parse(process.env.KAMACU_BROWSER_ARGS) : [])],
  });
  const directory = fs.mkdtempSync(path.join(os.tmpdir(), 'kamacu-artifact-agent-'));
  try {
    async function upload(title, filename, bytes, artifactID) {
      const endpoint = artifactID ? `/api/tasks/1/artifacts/${artifactID}/uploads` : '/api/tasks/1/artifact-uploads';
      let response = await fetch(base + endpoint, { method: 'POST', headers: { 'X-Kamacu-Token': 'test-token' },
        body: JSON.stringify({ title, filename, size_bytes: bytes.length }) });
      assert.equal(response.status, 201, await response.clone().text());
      const session = await response.json();
      response = await fetch(base + session.upload_url, { method: session.method, headers: session.headers, body: bytes });
      assert.equal(response.status, 204);
      response = await fetch(`${base}/api/tasks/1/artifact-uploads/${session.upload_id}/finalize`, {
        method: 'POST', headers: { 'X-Kamacu-Token': 'test-token' },
      });
      assert.equal(response.status, 200, await response.clone().text());
      return response.json();
    }
    const page = await browser.newPage({ viewport: { width: 1440, height: 900 } });
    const pageErrors = [];
    page.on('pageerror', error => { pageErrors.push(error.message); console.error('PAGE ERROR:', error.message); });
    // CSP refusal messages are expected; uncaught JavaScript errors are not.
    const task = { id: 1, project_id: 1, title: 'Preview artifacts', description: 'Interactive task previews',
      status: 'todo', position: 0, source: 'manual', worktree_path: null, recurrent: false,
      created_at: '2026-10-22T00:00:00Z', updated_at: '2026-10-22T00:00:00Z' };
    await page.route('**/api/**', async route => {
      const pathname = new URL(route.request().url()).pathname;
      if (/artifact/.test(pathname)) return route.continue();
      let body = [];
      if (pathname === '/api/workspaces') body = [{ id: 1, name: 'Personal', is_default: true }];
      else if (pathname === '/api/projects') body = [{ id: 1, name: 'Kamacu', workspace_id: 1, agent_id: 1, repo_path: '/tmp/example', description: '', github_repo: null, managed: false, icon_letters: 'KA', icon_color: 'blue' }];
      else if (pathname === '/api/agents') body = [{ id: 1, name: 'Test', engine: 'custom', command: 'test' }];
      else if (pathname === '/api/tasks/1') body = task;
      else if (pathname === '/api/settings') body = {};
      else if (pathname === '/api/global') body = { root_path: '', root_exists: false, live: { agent: 0, bash: 0, tmux: 0 }, agent: { id: 1, engine: 'custom', name: 'Test' } };
      await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(body) });
    });
    await page.goto(`${base}/projects/1/tasks/1`);
    await page.getByRole('tab', { name: 'Agent', exact: true }).waitFor({timeout: 10000}).catch(async error => { console.error((await page.locator('body').innerText()).slice(0, 2000)); throw error; });
    const html = Buffer.from(`<!doctype html><title>Preview</title><h1>Interactive preview</h1><input aria-label="Note"><button onclick="document.querySelector('output').textContent=Number(document.querySelector('output').textContent)+1">Count</button><output>0</output>
      <script>
      document.body.dataset.inline='yes';
      try {
        const peer = new RTCPeerConnection({iceServers:[{urls:'stun:127.0.0.1:${stun.address().port}'}]});
        peer.createDataChannel('probe');
        peer.createOffer().then(offer=>peer.setLocalDescription(offer)).catch(()=>{});
        setTimeout(()=>peer.close(), 3000);
      } catch { document.body.dataset.rtcBlocked='yes'; }
      try { parent.document.body.dataset.escaped='yes'; } catch { document.body.dataset.parentBlocked='yes'; }
      try { localStorage.setItem('escape','yes'); } catch { document.body.dataset.storageBlocked='yes'; }
      fetch('${outside}/fetch').catch(()=>{});
      fetch('/api/tasks/1').catch(()=>{});
      try { new WebSocket('ws://127.0.0.1:${external.address().port}/socket'); } catch {}
      </script><img src="${outside}/image"><script src="${outside}/script"></script><link rel="stylesheet" href="${outside}/style">`);
    const first = await upload('Calculator', 'preview.html', html);
    const tab = page.getByRole('tab', { name: 'Calculator', exact: true });
    await tab.waitFor();
    assert.equal(await page.getByRole('tab', { name: 'Agent', exact: true }).getAttribute('aria-selected'), 'true', 'new artifact stole focus');
    assert.equal(await page.locator('iframe').count(), 0, 'unopened artifact executed');
    await tab.click();
    const frame = page.frameLocator('iframe').first();
    if (process.env.KAMACU_EXPECT_UNSUPPORTED === '1') {
      // No uploaded markup may execute or even reach the HTML parser on an
      // engine that ignores native network containment. Direct URL navigation
      // must fail closed as well as the task tab.
      await page.getByRole('alert').filter({ hasText: 'cannot enforce' }).waitFor();
      assert.equal(await page.locator('iframe[title="Calculator"]').count(), 0);
      const direct = await browser.newPage();
      await direct.goto(`${base}/api/artifact-content/${first.id}/${first.revision}/index.html`);
      await direct.getByText('This browser cannot enforce', {exact:false}).waitFor();
      assert.equal(await direct.locator('h1').count(), 0);
      assert.equal(await direct.locator('body').getAttribute('data-inline'), null);
      assert.deepEqual(externalHits, []);
      assert.equal(stunPackets, 0);
      await direct.close();
      fs.writeFileSync(path.join(directory, 'index.html'), '<h1>Assets</h1>');
      fs.writeFileSync(path.join(directory, 'active.svg'), `<svg xmlns="http://www.w3.org/2000/svg"><text>Static asset</text><script>document.documentElement.setAttribute('data-ran','yes'); new RTCPeerConnection();</script><a href="${outside}/svg-navigation"><text>External link</text></a></svg>`);
      fs.writeFileSync(path.join(directory, 'active.xhtml'), `<html xmlns="http://www.w3.org/1999/xhtml"><body><script>document.documentElement.setAttribute('data-ran','yes'); new RTCPeerConnection();</script><a href="${outside}/xhtml-navigation">External link</a></body></html>`);
      const activeArchive = execFileSync('tar', ['-czf', '-', '-C', directory, 'index.html', 'active.svg', 'active.xhtml']);
      const active = await upload('Active assets', 'active.tar.gz', activeArchive);
      for (const filename of ['active.svg','active.xhtml']) {
        const asset = await browser.newPage();
        const download = asset.waitForEvent('download');
        await asset.goto(`${base}/api/artifact-content/${active.id}/${active.revision}/${filename}`)
          .catch(error => assert.match(error.message, /download|ERR_ABORTED/i));
        const received = await download;
        assert.equal(received.suggestedFilename(), filename);
        await received.cancel();
        assert.equal(await asset.locator(':root').getAttribute('data-ran'), null, `${filename} executed on unsupported browser`);
        await asset.close();
      }
      console.log('PASS: unsupported browser fails closed in task tab and direct navigation; no uploaded code, HTTP traffic, or STUN packets');
      return;
    }
    await frame.getByRole('heading', { name: 'Interactive preview' }).waitFor();
    assert.equal(await frame.locator('#preview-status').count(), 0, 'wrapper status remained above the artifact');
    assert.equal(await frame.locator('body').getAttribute('data-inline'), 'yes');
    assert.equal(await frame.locator('body').getAttribute('data-parent-blocked'), 'yes');
    assert.equal(await frame.locator('body').getAttribute('data-storage-blocked'), 'yes');
    await frame.getByRole('textbox', { name: 'Note' }).fill('Keep this state');
    await frame.getByRole('button', { name: 'Count' }).click();
    await page.getByRole('tab', { name: 'Description', exact: true }).click();
    await tab.click();
    assert.equal(await frame.getByRole('textbox', { name: 'Note' }).inputValue(), 'Keep this state');
    assert.equal(await frame.locator('output').textContent(), '1');
    // User title changes survive subsequent agent replacement.
    await tab.locator('span').first().dblclick();
    const titleInput = page.getByRole('tab').getByRole('textbox');
    await titleInput.fill('My calculator');
    await titleInput.press('Enter');
    await page.getByRole('tab', { name: 'My calculator', exact: true }).waitFor();
    const replacement = await upload('', 'preview.html', Buffer.from('<h1>Updated preview</h1>'), first.id);
    assert.equal(replacement.title, 'My calculator');
    await page.getByRole('button', { name: 'Reload', exact: true }).waitFor();
    assert.equal(await frame.getByRole('textbox', { name: 'Note' }).inputValue(), 'Keep this state');
    await page.getByRole('button', { name: 'Reload', exact: true }).click();
    await page.frameLocator('iframe').getByRole('heading', { name: 'Updated preview' }).waitFor();
    // Real tar.gz upload with local assets, module script, and local JSON fetch.
    fs.writeFileSync(path.join(directory, 'index.html'), `<!doctype html><link rel="stylesheet" href="style.css"><h1>Bundled preview</h1><img alt="Bundled image" src="image.svg"><script src="classic.js"></script><script type="module" src="module.js"></script>`);
    fs.writeFileSync(path.join(directory, 'image.svg'), '<svg xmlns="http://www.w3.org/2000/svg" width="40" height="20"><rect width="40" height="20" fill="blue"/></svg>');
    fs.writeFileSync(path.join(directory, 'style.css'), 'h1 { color: rgb(12, 34, 56); }');
    fs.writeFileSync(path.join(directory, 'classic.js'), "document.body.dataset.classic='yes'");
    fs.writeFileSync(path.join(directory, 'module.js'), "import {value} from './dep.js'; document.body.dataset.module=value; fetch('./data.json').then(r=>r.json()).then(v=>document.body.dataset.localFetch=v.value)");
    fs.writeFileSync(path.join(directory, 'dep.js'), "export const value='yes'");
    fs.writeFileSync(path.join(directory, 'data.json'), '{"value":"yes"}');
    const archive = execFileSync('tar', ['-czf', '-', '-C', directory, 'index.html', 'style.css', 'classic.js', 'module.js', 'dep.js', 'data.json', 'image.svg']);
    await upload('Bundle', 'bundle.tar.gz', archive);
    const bundleTab = page.getByRole('tab', { name: 'Bundle', exact: true });
    await bundleTab.waitFor(); await bundleTab.click();
    const bundleFrame = page.frameLocator('iframe[title="Bundle"]');
    await bundleFrame.getByRole('heading', { name: 'Bundled preview' }).waitFor();
    await bundleFrame.locator('body[data-module="yes"][data-classic="yes"][data-local-fetch="yes"]').waitFor();
    assert.equal(await bundleFrame.locator('h1').evaluate(el => getComputedStyle(el).color), 'rgb(12, 34, 56)');
    await bundleFrame.getByRole('img', {name:'Bundled image'}).evaluate(image => image.decode());
    assert.equal(await bundleFrame.getByRole('img', {name:'Bundled image'}).evaluate(image => image.naturalWidth), 40);
    assert.deepEqual(externalHits, [], 'external resources or requests escaped network policy');
    assert.equal(stunPackets, 0, 'WebRTC/STUN traffic escaped network policy');
    if (process.env.KAMACU_ARTIFACT_SCREENSHOTS) {
      fs.mkdirSync(process.env.KAMACU_ARTIFACT_SCREENSHOTS, { recursive: true });
      await page.screenshot({ path: path.join(process.env.KAMACU_ARTIFACT_SCREENSHOTS, 'artifacts-desktop.png') });
      await page.setViewportSize({ width: 390, height: 844 });
      await page.screenshot({ path: path.join(process.env.KAMACU_ARTIFACT_SCREENSHOTS, 'artifacts-mobile.png') });
      await page.setViewportSize({ width: 1440, height: 900 });
    }
    // Worktree is absent throughout this test; previews remain available.
    await bundleTab.getByRole('button', { name: 'Remove Bundle', exact: true }).click();
    await page.getByRole('button', { name: 'Cancel', exact: true }).click();
    await bundleTab.waitFor();
    await bundleTab.getByRole('button', { name: 'Remove Bundle', exact: true }).click();
    await page.getByRole('button', { name: 'Remove artifact', exact: true }).click();
    await bundleTab.waitFor({ state: 'detached' });
    assert.equal(await page.getByRole('tab', { name: 'My calculator', exact: true }).getAttribute('aria-selected'), 'true');
    await page.reload();
    await page.getByRole('tab', { name: 'My calculator', exact: true }).click();
    await page.frameLocator('iframe').getByRole('heading', { name: 'Updated preview' }).waitFor();
    assert.deepEqual(pageErrors, []);
    // The native policy must cover navigation as well as WebRTC. Uploaded
    // code has already attempted STUN from the original artifact document.
    const nav = await upload('Navigation probe', 'navigation.html', Buffer.from(`<h1>Navigation probe</h1><button onclick="location.href='${outside}/navigation'">Navigate externally</button>`));
    await page.getByRole('tab', { name: 'Navigation probe', exact: true }).click();
    const navFrame = page.frameLocator('iframe[title="Navigation probe"]');
    await navFrame.getByRole('button', { name: 'Navigate externally' }).click();
    await page.getByRole('tab', { name: 'My calculator', exact: true }).click();
    assert.deepEqual(externalHits, [], 'external self-navigation escaped native policy');
    assert.equal(stunPackets, 0, 'WebRTC/STUN traffic escaped native policy');
    await fetch(`${base}/api/tasks/1/artifacts/${nav.id}`, { method: 'DELETE' });
    // HTTP and transport failures are explicit, retryable, and do not require
    // a new revision. First fail with 404, then a connection refusal, then heal.
    const retry = await upload('Retry probe', 'retry.html', Buffer.from('<h1>Retry works</h1>'));
    let failure = '404';
    let checks = 0;
    const retryRoute = `**/api/artifact-content/${retry.id}/${retry.revision}/index.html*`;
    await page.route(retryRoute, async route => {
      if (route.request().method() === 'HEAD') checks++;
      if (failure === '404') return route.fulfill({status:404,contentType:'text/plain',body:'not found'});
      if (failure === 'network') return route.abort('connectionrefused');
      if (failure === 'get404' && route.request().method() !== 'HEAD') return route.fulfill({status:404,contentType:'text/plain',body:'not found'});
      if (failure === 'getnetwork' && route.request().method() !== 'HEAD') return route.abort('connectionrefused');
      return route.continue();
    });
    await page.getByRole('tab', { name: 'Retry probe', exact: true }).click();
    await page.getByRole('alert').filter({ hasText:'no longer available' }).waitFor();
    assert.equal(await page.locator('iframe[title="Retry probe"]').count(), 0);
    if (process.env.KAMACU_ARTIFACT_SCREENSHOTS) {
      await page.screenshot({ path:path.join(process.env.KAMACU_ARTIFACT_SCREENSHOTS,'retry-error-desktop.png') });
      await page.setViewportSize({width:390,height:844});
      await page.screenshot({ path:path.join(process.env.KAMACU_ARTIFACT_SCREENSHOTS,'retry-error-mobile.png') });
      await page.setViewportSize({width:1440,height:900});
    }
    failure = 'network';
    await page.getByRole('button', { name:'Retry', exact:true }).click();
    await page.getByRole('alert').waitFor();
    // HEAD can succeed and the actual iframe navigation still fail. An iframe
    // load event alone must not clear readiness failure or defeat Retry.
    for (const nextFailure of ['get404','getnetwork']) {
      failure = nextFailure;
      await page.getByRole('button', { name:'Retry', exact:true }).click();
      await page.getByRole('alert').filter({hasText:/took too long|could not be loaded/}).waitFor();
    }
    failure = '';
    await page.getByRole('button', { name:'Retry', exact:true }).click();
    await page.frameLocator('iframe[title="Retry probe"]').getByRole('heading', { name:'Retry works' }).waitFor();
    assert.equal(checks, 5, 'retry did not recheck the same revision');
    assert.deepEqual(externalHits, []);
    assert.equal(stunPackets, 0);
    assert.deepEqual(pageErrors, []);
    console.log('PASS: remote HTTP upload, HTML and tar.gz, local scripts/modules/CSS/fetch, parent/storage isolation, external HTTP/navigation/WebRTC blocking, tabs/state/update notice, rename, remove, reload, HTTP/transport failure retry, desktop/mobile');
  } finally {
    fs.rmSync(directory, { recursive: true, force: true });
    await browser.close();
    await new Promise(resolve => external.close(resolve));
    await new Promise(resolve => stun.close(resolve));
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
