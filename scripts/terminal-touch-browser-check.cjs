// Run against a local Vite server with the browser overrides used by other checks.
const { chromium } = require(process.env.KAMACU_PLAYWRIGHT_PACKAGE || 'playwright');
const assert = require('node:assert/strict');

(async () => {
  const base = process.argv[2];
  assert.ok(base, 'Pass a local Vite server URL');
  const browser = await chromium.launch({ headless: true, executablePath: process.env.KAMACU_BROWSER_EXECUTABLE, args: ['--no-sandbox'] });
  try {
    const page = await browser.newPage({ viewport: { width: 390, height: 844 }, deviceScaleFactor: 3, isMobile: true, hasTouch: true });
    const errors = [];
    page.on('pageerror', error => errors.push(error.message));
    await page.route('**/terminal-touch-check', route => route.fulfill({
      contentType: 'text/html',
      body: '<meta name="viewport" content="width=device-width,initial-scale=1"><link rel="stylesheet" href="/node_modules/@xterm/xterm/css/xterm.css"><div id="terminal" style="width:370px;height:500px;touch-action:none"></div>',
    }));
    await page.goto(base + '/terminal-touch-check');
    await page.evaluate(async () => {
      const [{ Terminal }, { attachTwoFingerScroll }, { scrollTerminal }] = await Promise.all([
        import('/node_modules/@xterm/xterm/lib/xterm.mjs'),
        import('/src/components/terminal/touchScroll.ts'),
        import('/src/components/terminal/scrollTerminal.ts'),
      ]);
      const element = document.getElementById('terminal');
      const term = new Terminal({ cols: 40, rows: 25, scrollback: 1000 });
      term.open(element);
      const input = [];
      term.onData(data => input.push(data));
      const detach = attachTwoFingerScroll(element, (lines, position) => scrollTerminal(term, lines, position), () => element.querySelector('.xterm-screen').getBoundingClientRect().height / term.rows);
      window.harness = { term, input, detach };
      await new Promise(resolve => term.write(Array.from({ length: 200 }, (_, index) => `history ${index}\r\n`).join(''), resolve));
    });
    const cdp = await page.context().newCDPSession(page);
    async function drag(fromY, toY) {
      // Land one finger before the other, as on a real phone, then lift them
      // separately. No click, selection, or app input should leak through.
      const first = y => ({ id: 1, x: 80, y });
      const second = y => ({ id: 2, x: 140, y });
      await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [first(fromY)] });
      await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [first(fromY), second(fromY)] });
      for (let step = 1; step <= 10; step++) {
        const y = fromY + (toY - fromY) * step / 10;
        await cdp.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [first(y), second(y)] });
        await page.waitForTimeout(16);
      }
      await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [first(toY)] });
      await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
      await page.waitForTimeout(100);
    }
    const initial = await page.evaluate(() => window.harness.term.buffer.active.viewportY);
    await drag(150, 300);
    const older = await page.evaluate(() => window.harness.term.buffer.active.viewportY);
    assert.ok(older < initial, 'Dragging down reveals older shell history');
    await drag(300, 150);
    assert.ok(await page.evaluate(() => window.harness.term.buffer.active.viewportY) > older, 'Dragging up returns to newer history');
    assert.deepEqual(await page.evaluate(() => window.harness.input), [], 'Shell history scrolling sends no input');

    // tmux enables alternate screen + mouse tracking; scrolling must reach
    // its history instead of the empty local alternate-buffer scrollback.
    await page.evaluate(() => new Promise(resolve => window.harness.term.write('\x1b[?1049h\x1b[?1000h\x1b[?1006h', resolve)));
    await drag(150, 300);
    const mouseUp = await page.evaluate(() => window.harness.input.splice(0));
    assert.ok(mouseUp.length > 1 && mouseUp.every(data => /^\x1b\[<64;\d+;\d+M$/.test(data)), 'tmux receives wheel-up reports without stray clicks or keys');
    await drag(300, 150);
    const mouseDown = await page.evaluate(() => window.harness.input.splice(0));
    assert.ok(mouseDown.length > 1 && mouseDown.every(data => /^\x1b\[<65;\d+;\d+M$/.test(data)), 'tmux receives wheel-down reports');

    await page.evaluate(() => new Promise(resolve => window.harness.term.write('\x1b[?1000l\x1b[?1h', resolve)));
    await drag(150, 300);
    const cursors = await page.evaluate(() => window.harness.input.splice(0));
    assert.ok(cursors.length > 1 && cursors.every(data => data === '\x1bOA'), 'Alternate-screen apps without mouse tracking receive application cursor keys');
    assert.equal(await page.evaluate(() => window.scrollY), 0, 'Gesture does not pan the page');
    assert.equal(await page.evaluate(() => window.visualViewport.scale), 1, 'Gesture does not zoom the page');
    await cdp.detach();
    assert.deepEqual(errors, []);
    console.log('Two-finger shell, tmux mouse, and alternate-screen scrolling checks passed');
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exit(1); });
