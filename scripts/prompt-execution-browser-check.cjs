// Run against Vite. Optionally save captured stdin for prompt-execution-cli-check.py:
// node scripts/prompt-execution-browser-check.cjs http://127.0.0.1:5187 /tmp/prompt-frames.json
const { chromium } = require(process.env.KAMACU_PLAYWRIGHT_PACKAGE || 'playwright');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');

(async () => {
  const base = process.argv[2];
  assert.ok(base, 'Pass a local Vite server URL');
  const filename = `__prompt-execution-${process.pid}.tsx`;
  const harness = path.join(__dirname, '../web/src', filename);
  const description = 'Reply with DESCRIPTION_OK.';
  const review = 'Reply with REVIEW_OK.\nThis is a multiline review prompt.';
  const prompts = [
    { title: 'Multiline prompt', text: 'Reply with MULTI_OK.\nThis is the second line: café 🚀.' },
    { title: 'Long prompt', text: 'Reply with LONG_OK.\n' + 'Preserve this line. '.repeat(600) + 'End.' },
  ];
  fs.writeFileSync(harness, `
import React from 'react';
import { createRoot } from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import { TooltipProvider } from './components/ui/tooltip';
import { AgentTab } from './components/task/AgentTab';
import './index.css';
const client = new QueryClient({defaultOptions: {queries: {staleTime: Infinity, retry: false}}});
client.setQueryData(['task', 1], {id: 1, worktree_path: '/tmp/prompt-test', branch: 'test'});
client.setQueryData(['global'], {});
client.setQueryData(['agent-statuses'], []);
client.setQueryData(['settings'], {custom_prompts: {value: ${JSON.stringify(JSON.stringify(prompts))}}});
const root = createRoot(document.getElementById('root')!);
Object.assign(window, { renderAgent: (engine: string) => root.render(
  <QueryClientProvider client={client}><TooltipProvider>
    <div style={{height: 600}}><AgentTab scope={{kind: 'task', taskId: 1}}
      agentSession={{id: engine, engine, status: 'running', label: 'Agent', createdAt: ''}}
      description={${JSON.stringify(description)}} seed={${JSON.stringify(review)}} /></div>
  </TooltipProvider></QueryClientProvider>
)});
`, { flag: 'wx' });
  let browser;
  try {
    browser = await chromium.launch({ headless: true, executablePath: process.env.KAMACU_BROWSER_EXECUTABLE, args: ['--no-sandbox'] });
    const page = await browser.newPage();
    page.setDefaultTimeout(10000);
    const errors = [];
    const captured = [];
    let frames = [];
    let rawInput = [];
    let socket;
    let closeAfterPaste = false;
    page.on('pageerror', error => { errors.push(error.message); console.error(error.stack); });
    await page.route('**/__prompt-execution-check', route => route.fulfill({ contentType: 'text/html', body: `
<html><head><script type="module">
import { injectIntoGlobalHook } from '/@react-refresh';
injectIntoGlobalHook(window); window.$RefreshReg$ = () => {}; window.$RefreshSig$ = () => type => type;
</script></head><body><div id="root"></div><script type="module" src="/src/${filename}"></script></body></html>` }));
    await page.route('**/api/**', route => {
      const pathname = new URL(route.request().url()).pathname;
      if (!pathname.startsWith('/api/')) return route.continue();
      return route.fulfill({ json: pathname === '/api/tasks/1'
        ? {id: 1, worktree_path: '/tmp/prompt-test', branch: 'test'} : pathname === '/api/global' ? {} : [] });
    });
    await page.routeWebSocket('**/api/sessions/*/ws', ws => {
      socket = ws;
      ws.onMessage(message => {
        const bytes = Buffer.from(message);
        if (bytes[0] !== 0x30) return;
        const data = bytes.subarray(1).toString();
        rawInput.push(data);
        // Automatic terminal reports are unrelated to prompt input.
        if (!data.startsWith('\x1b[200~') && data !== '\r') return;
        frames.push({ data, ms: Date.now() });
        if (closeAfterPaste && data.startsWith('\x1b[200~')) ws.close({code: 1011, reason: 'test disconnect'});
      });
      ws.send(Buffer.concat([Buffer.from([0x30]), Buffer.from('\x1b[?2004hReady\r\n')]));
    });
    await page.goto(base + '/__prompt-execution-check');
    await page.waitForFunction(() => typeof window.renderAgent === 'function');
    for (const engine of ['claude', 'codex']) {
      await page.evaluate(engine => window.renderAgent(engine), engine);
      await page.locator('.xterm-helper-textarea').waitFor({state: 'attached'});
      await page.waitForTimeout(150);
      for (const [title, text] of [['Insert description', description], ['Insert review prompt', review], ...prompts.map(prompt => [prompt.title, prompt.text])]) {
        frames = [];
        await page.getByRole('button', {name: 'Agent actions', exact: true}).click();
        await page.getByRole('menuitem', {name: title, exact: true}).click();
        await page.waitForTimeout(250);
        assert.equal(frames.length, 2, `${engine}/${title}: exactly one paste and one Enter`);
        assert.equal(frames[0].data, '\x1b[200~' + text.replace(/\r?\n/g, '\r') + '\x1b[201~');
        assert.equal(frames[1].data, '\r');
        assert.ok(frames[1].ms - frames[0].ms >= 90, 'Enter is separate from the paste');
        captured.push({engine, title, text, frames: frames.map(frame => ({...frame, ms: frame.ms - frames[0].ms}))});
      }
    }
    socket.send(Buffer.concat([Buffer.from([0x30]), Buffer.from('\x1b[?2004l')]));
    await page.waitForTimeout(50);
    frames = [];
    rawInput = [];
    await page.getByRole('button', {name: 'Agent actions', exact: true}).click();
    await page.getByRole('menuitem', {name: 'Multiline prompt', exact: true}).click();
    await page.waitForTimeout(150);
    assert.deepEqual(rawInput, [], 'No raw multiline input before the CLI enables bracketed paste');
    assert.match(await page.getByRole('alert').innerText(), /Prompt not sent/);
    socket.send(Buffer.concat([Buffer.from([0x30]), Buffer.from('\x1b[?2004h')]));
    await page.waitForTimeout(50);
    await page.getByRole('button', {name: 'Agent actions', exact: true}).click();
    await page.getByRole('menuitem', {name: 'Multiline prompt', exact: true}).click();
    await page.waitForTimeout(250);
    assert.equal(frames.length, 2, 'Retry succeeds once the CLI is ready');
    assert.equal(await page.getByRole('alert').count(), 0, 'Successful retry clears the message');
    await page.getByRole('button', {name: 'Show helper keyboard', exact: true}).click();
    for (const manualInput of ['Enter', 'keyboard edit', 'paste', 'mobile input', 'helper Enter']) {
      frames = [];
      await page.getByRole('button', {name: 'Agent actions', exact: true}).click();
      // Trigger input immediately after the menu action, before its timer can
      // fire. This runs in the browser to avoid automation round-trip timing.
      await page.evaluate(mode => {
        document.addEventListener('click', () => queueMicrotask(() => {
          const textarea = document.querySelector('.xterm-helper-textarea');
          if (mode === 'helper Enter') {
            document.querySelector('button[aria-label="Enter"]').click();
          } else if (mode === 'paste') {
            const clipboardData = new DataTransfer();
            clipboardData.setData('text/plain', 'Manual paste');
            textarea.dispatchEvent(new ClipboardEvent('paste', {clipboardData, bubbles: true, cancelable: true}));
          } else if (mode === 'mobile input') {
            textarea.value = 'x';
            textarea.dispatchEvent(new InputEvent('input', {data: 'x', inputType: 'insertText', bubbles: true}));
          } else {
            const enter = mode === 'Enter';
            textarea.dispatchEvent(new KeyboardEvent('keydown', {key: enter ? 'Enter' : 'x', code: enter ? 'Enter' : 'KeyX', keyCode: enter ? 13 : 88, bubbles: true, cancelable: true}));
          }
        }), {once: true});
      }, manualInput);
      await page.getByRole('menuitem', {name: 'Multiline prompt', exact: true}).click();
      await page.waitForTimeout(250);
      assert.equal(frames[0].data.startsWith('\x1b[200~'), true, 'The configured prompt is pasted first');
      assert.equal(frames.filter(frame => frame.data === '\r').length,
        manualInput.endsWith('Enter') ? 1 : 0, `${manualInput}: automatic Enter is canceled`);
    }
    frames = [];
    closeAfterPaste = true;
    await page.getByRole('button', {name: 'Agent actions', exact: true}).click();
    await page.getByRole('menuitem', {name: 'Multiline prompt', exact: true}).click();
    await page.waitForTimeout(300);
    assert.equal(frames.length, 1, 'A dropped connection cancels the pending Enter');
    closeAfterPaste = false;
    await page.waitForTimeout(700);
    assert.equal(frames.length, 1, 'Reconnect must not send a stale Enter');
    socket.close();
    assert.deepEqual(errors, []);
    if (process.argv[3]) fs.writeFileSync(process.argv[3], JSON.stringify(captured, null, 2));
    console.log('PASS: prompt actions paste and submit; physical keys, clipboard, mobile input, helper keys, and disconnect cancel pending Enter.');
  } finally {
    if (browser) await browser.close();
    fs.unlinkSync(harness);
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
