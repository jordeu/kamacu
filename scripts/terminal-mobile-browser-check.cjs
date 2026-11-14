// Run against a local Vite server; use the same browser overrides as other checks.
const { chromium } = require(process.env.KAMACU_PLAYWRIGHT_PACKAGE || 'playwright');
const assert = require('node:assert/strict');

(async () => {
  const base = process.argv[2];
  assert.ok(base, 'Pass a local Vite server URL');
  const browser = await chromium.launch({ headless: true, executablePath: process.env.KAMACU_BROWSER_EXECUTABLE, args: ['--no-sandbox'] });
  try {
    const page = await browser.newPage({ viewport: { width: 390, height: 844 }, isMobile: true, hasTouch: true });
    const errors = [];
    const input = [];
    const sizes = [];
    let attachments = 0;
    let socket;
    page.on('pageerror', error => { errors.push(error.message); console.error(error.message); });
    await page.route('**/api/**', route => {
      const path = new URL(route.request().url()).pathname;
      if (!path.startsWith('/api/')) return route.continue();
      if (path === '/api/events' || path === '/api/pending') return route.fulfill({ contentType: 'text/event-stream', body: ': mock\n\n' });
      const data = path === '/api/sessions' ? [{ id: 'mobile-shell', label: 'bash #1', status: 'running', createdAt: '2026-11-14T10:00:00Z' }]
        : path === '/api/workspaces' ? [{ id: 1, name: 'Work' }]
        : path === '/api/settings' ? {}
        : path === '/api/inbox' ? { items: [], pending_count: 0, has_more: false }
        : path === '/api/inbox/count' ? { count: 0 } : [];
      return route.fulfill({ json: data });
    });
    await page.routeWebSocket('**/api/sessions/*/ws', ws => {
      attachments++;
      socket = ws;
      ws.onMessage(message => {
        const data = Buffer.from(message);
        if (data[0] === 0x30) input.push(data.subarray(1).toString());
        if (data[0] === 0x31) sizes.push(JSON.parse(data.subarray(1).toString()));
      });
      ws.send(Buffer.concat([Buffer.from([0x30]), Buffer.from('Mobile terminal ready\r\n$ ')]));
    });
    await page.goto(base + '/terminal');
    await page.getByRole('button', { name: /bash #1/ }).click();
    await page.locator('.xterm-helper-textarea').waitFor({ state: 'attached' });
    const fullscreen = page.getByRole('button', { name: 'Full screen terminal', exact: true });
    const pane = fullscreen.locator('xpath=../../..');
    const initialBox = await pane.boundingBox();
    assert.ok(initialBox.width > 340, 'Session rail leaves room for the terminal on mobile');
    await fullscreen.tap();
    await page.waitForFunction(() => document.querySelector('button[aria-label="Exit full screen"]')?.closest('.fixed')?.getBoundingClientRect().height === innerHeight);
    assert.ok(await page.locator('nav[aria-label="Mobile navigation"]').evaluate(element => {
      const link = element.querySelector('a');
      link.focus();
      return element.closest('[inert]') !== null && document.activeElement !== link;
    }), 'Covered navigation cannot receive focus while fullscreen');
    await page.waitForTimeout(200);
    assert.ok(sizes.length > 0, 'Terminal sends PTY dimensions after connecting');
    const fullscreenRows = sizes.at(-1).rows;
    const attachmentCount = attachments;
    await page.getByRole('button', { name: 'Show helper keyboard' }).tap();
    const keyboard = page.getByRole('group', { name: 'Terminal helper keyboard' });
    await keyboard.waitFor();
    await page.waitForTimeout(200);
    assert.ok(sizes.at(-1).rows < fullscreenRows, 'Showing helper keys resizes the PTY');
    const textarea = page.locator('.xterm-helper-textarea');
    await textarea.focus();
    for (const [name, data] of [['Escape', '\x1b'], ['Tab', '\t'], ['Arrow up', '\x1b[A'], ['Ctrl+C: interrupt', '\x03']]) {
      const key = keyboard.getByRole('button', { name, exact: true });
      await key.tap();
      await page.waitForFunction(() => document.activeElement?.classList.contains('xterm-helper-textarea'));
      await new Promise(resolve => setTimeout(resolve, 50));
      assert.equal(input.at(-1), data, `${name} sends raw terminal input`);
    }
    socket.send(Buffer.concat([Buffer.from([0x30]), Buffer.from('\x1b[?1h')]));
    await page.waitForTimeout(100);
    await keyboard.getByRole('button', { name: 'Arrow up', exact: true }).tap();
    await page.waitForTimeout(100);
    assert.equal(input.at(-1), '\x1bOA', 'TUI application cursor mode is respected');

    // Use native touch events: synthetic scrollLeft changes miss accidental
    // key presses or prevented scrolling when a swipe starts on a key.
    const row = keyboard.locator('div').last();
    const rowBox = await row.boundingBox();
    const cdp = await page.context().newCDPSession(page);
    const startX = rowBox.x + rowBox.width - 25;
    const y = rowBox.y + rowBox.height / 2;
    const beforeSwipe = input.length;
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchStart', touchPoints: [{ x: startX, y }] });
    for (let step = 1; step <= 8; step++) {
      await cdp.send('Input.dispatchTouchEvent', { type: 'touchMove', touchPoints: [{ x: startX - step * 30, y }] });
      await page.waitForTimeout(20);
    }
    await cdp.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
    await page.waitForTimeout(200);
    assert.ok(await row.evaluate(element => element.scrollLeft > 0), 'Key rows can be swiped');
    assert.equal(input.length, beforeSwipe, 'Swiping over keys sends no input');
    await cdp.detach();
    await keyboard.getByRole('button', { name: 'Delete', exact: true }).tap();
    await page.waitForTimeout(100);
    assert.equal(input.at(-1), '\x1b[3~', 'Offscreen keys remain reachable');
    await page.getByRole('button', { name: 'Hide helper keyboard' }).tap();
    await keyboard.waitFor({ state: 'hidden' });
    await page.getByRole('button', { name: 'Show helper keyboard' }).tap();
    await page.setViewportSize({ width: 390, height: 450 });
    await page.waitForTimeout(200);
    const resized = await page.getByRole('button', { name: 'Exit full screen' }).locator('xpath=../../..').boundingBox();
    assert.equal(resized.height, 450, 'Fullscreen follows keyboard-sized viewport');
    const helperBox = await keyboard.boundingBox();
    assert.ok(helperBox.y + helperBox.height <= 450, 'Helper keyboard stays visible');
    await page.setViewportSize({ width: 844, height: 180 });
    await page.waitForTimeout(200);
    const shortKeyboard = await keyboard.boundingBox();
    assert.ok(shortKeyboard.y + shortKeyboard.height <= 180, 'Helper keyboard fits a short landscape viewport');
    const terminalHeight = await page.locator('.xterm').evaluate(element => element.parentElement.clientHeight);
    assert.ok(terminalHeight >= 35, 'Terminal remains usable above helper keys in landscape');
    assert.ok(await keyboard.evaluate(element => element.scrollHeight > element.clientHeight), 'Helper keys scroll vertically when space is limited');
    await page.setViewportSize({ width: 844, height: 390 });
    await page.waitForTimeout(200);
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false, 'Landscape has no page overflow');
    await page.setViewportSize({ width: 320, height: 568 });
    await page.waitForTimeout(200);
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth), false, 'Narrow phones have no page overflow');
    assert.equal(attachments, attachmentCount, 'Fullscreen and helper toggles preserve the connection');
    if (process.env.KAMACU_BROWSER_SCREENSHOTS) await page.screenshot({ path: process.env.KAMACU_BROWSER_SCREENSHOTS + '-mobile.png' });
    await page.getByRole('button', { name: 'Exit full screen' }).tap();
    await page.getByRole('button', { name: 'Full screen terminal', exact: true }).waitFor();
    assert.ok(await page.locator('nav[aria-label="Mobile navigation"]').evaluate(element => {
      const link = element.querySelector('a');
      link.focus();
      return element.closest('[inert]') === null && document.activeElement === link;
    }), 'Leaving fullscreen restores navigation');
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.getByRole('button', { name: 'Full screen terminal', exact: true }).click();
    await page.waitForTimeout(200);
    const desktop = await page.getByRole('button', { name: 'Exit full screen' }).locator('xpath=../../..').boundingBox();
    assert.equal(desktop.width, 1440);
    assert.equal(desktop.height, 1000);
    await page.setViewportSize({ width: 320, height: 568 });
    socket.send(Buffer.concat([Buffer.from([0x78]), Buffer.from('{"code":0}')]));
    await page.waitForFunction(() => document.querySelector('[aria-label="Ctrl+C: interrupt"]')?.disabled);
    const closeBox = await page.getByRole('button', { name: 'Close', exact: true }).boundingBox();
    assert.ok(closeBox.x >= 0 && closeBox.x + closeBox.width <= 320, 'Exited session actions fit narrow phones');
    assert.equal(errors.length, 0, errors.join('\n'));
    console.log('Terminal mobile/fullscreen checks passed');
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exit(1); });
