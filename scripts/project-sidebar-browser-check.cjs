// Run against Vite preview with API fixtures; no backend data is changed.
const { chromium } = require(process.env.KAMACU_PLAYWRIGHT_PACKAGE || 'playwright');
const assert = require('node:assert/strict');

(async () => {
  const browser = await chromium.launch({
    headless: true,
    executablePath: process.env.KAMACU_BROWSER_EXECUTABLE,
    args: ['--no-sandbox'],
  });
  try {
    const page = await browser.newPage({ viewport: { width: 1280, height: 600 } });
    const errors = [];
    page.on('pageerror', error => { errors.push(error.message); console.error('Page error:', error.message); });
    let projectCount = 40;
    let failPin = false;
    let holdPin = false;
    let releasePin;
    let pinRequests = 0;
    let notificationStatus = { pending_count: 0, processing: null };
    const projects = Array.from({ length: projectCount }, (_, index) => ({
      id: index + 1, name: index === 19 ? 'Long project name '.repeat(30) : `Project ${index + 1}`, workspace_id: 1,
      pinned: false, icon_letters: String(index + 1), icon_color: '#3b82f6',
      repo_path: '/tmp/project', description: '', github_repo: null, agent_id: 1,
    }));
    await page.route('**/api/**', async route => {
      const path = new URL(route.request().url()).pathname;
      let data = [];
      if (/^\/api\/projects\/\d+$/.test(path) && route.request().method() === 'PATCH') {
        pinRequests++;
        if (holdPin) await new Promise(resolve => { releasePin = resolve; });
        if (failPin) return route.fulfill({ status: 500, json: { error: 'Could not save pin' } });
        const project = projects[Number(path.split('/').pop()) - 1];
        Object.assign(project, route.request().postDataJSON());
        data = project;
      }
      else if (path === '/api/projects') data = [...projects.slice(0, projectCount), { ...projects[0], id: 99, name: 'Other workspace pin', workspace_id: 2, pinned: true }];
      else if (/^\/api\/projects\/\d+$/.test(path)) data = projects[Number(path.split('/').pop()) - 1];
      else if (path === '/api/workspaces') data = [{ id: 1, name: 'Work', is_default: true }];
      else if (path === '/api/inbox') data = { items: [], pending_count: 0, has_more: false };
      else if (path === '/api/notifications/status') data = notificationStatus;
      else if (path === '/api/notifications/rules') data = [{ id: 'rules', revision: 1, status: 'active', rules: [] }];
      else if (path === '/api/monitor') data = { profiles: [], default_profile_id: null, workspaces: [], usage: {} };
      else if (path === '/api/worktrees') data = { counts: { total: 0, orphaned: 0 }, projects: [] };
      else if (path === '/api/settings') data = Object.fromEntries(['worktree_base', 'shell', 'branch_template', 'done_session_ttl', 'github_integration', 'pr_review_seed'].map(key => [key, { value: '', default: '', source: 'default' }]));
      await route.fulfill({ json: data });
    });

    const nav = page.getByRole('navigation', { name: 'Projects', exact: true });
    const viewport = nav;
    const first = nav.getByRole('link', { name: 'Project 1', exact: true });
    const last = nav.getByRole('link', { name: 'Project 40', exact: true });
    async function isInside(link) {
      const outer = await viewport.boundingBox();
      const inner = await link.boundingBox();
      return inner.y >= outer.y - 1 && inner.y + inner.height <= outer.y + outer.height + 1;
    }
    async function checkOverflow() {
      assert.ok(await viewport.evaluate(el => el.scrollHeight > el.clientHeight));
      assert.equal(await viewport.evaluate(el => el.scrollWidth <= el.clientWidth), true, 'no horizontal scrolling');
      assert.equal(await viewport.evaluate(el => getComputedStyle(el).scrollbarWidth), 'none', 'native scrollbar is hidden');
      assert.equal(await viewport.evaluate(el => getComputedStyle(el, '::-webkit-scrollbar').display), 'none', 'WebKit scrollbar is hidden');
      assert.equal(await nav.locator('[data-orientation=vertical]').count(), 0, 'no custom scrollbar');
    }
    async function checkFooter() {
      const footer = page.locator('[data-sidebar=footer]:visible');
      const footerBounds = await footer.boundingBox();
      const navBounds = await nav.boundingBox();
      assert.ok(footerBounds.y >= navBounds.y + navBounds.height - 1, 'footer stays outside scroll region');
      assert.ok(footerBounds.y + footerBounds.height <= 600 - 36 + 1, 'footer clears sessions bar');
      await footer.getByRole('link', { name: 'Workspace overview', exact: true }).waitFor({ state: 'visible' });
      assert.equal(await footer.getByRole('link', { name: 'Activity', exact: true }).count(), 0);
    }

    await page.goto(process.argv[2] + '/settings');
    for (const expanded of [true, false]) {
      await page.evaluate(value => localStorage.setItem('kamacu.sidebar', String(value)), expanded);
      await page.goto(process.argv[2] + '/settings');
      await last.waitFor({ state: 'attached' });
      console.log('Checking', expanded ? 'expanded' : 'collapsed', await viewport.evaluate(el => ({ width: el.clientWidth, contentWidth: el.scrollWidth, height: el.clientHeight, contentHeight: el.scrollHeight })));
      await page.screenshot({ path: `/tmp/project-sidebar-${expanded ? 'expanded' : 'collapsed'}.png` });
      await checkOverflow();
      await checkFooter();
      assert.equal(await isInside(last), false, 'last project begins below viewport');
      await nav.focus();
      await page.keyboard.press('End');
      await page.waitForFunction(() => {
        const el = document.querySelector('nav[aria-label=Projects]');
        return el.scrollTop + el.clientHeight >= el.scrollHeight - 1;
      });
      assert.ok(await isInside(last), 'End reveals the last project without a scrollbar');
      await page.keyboard.press('Home');
      await page.waitForFunction(() => document.querySelector('nav[aria-label=Projects]').scrollTop === 0);
      const navBounds = await nav.boundingBox();
      await page.mouse.move(navBounds.x + navBounds.width / 2, navBounds.y + navBounds.height / 2);
      await page.mouse.wheel(0, 3000);
      await page.waitForFunction(() => {
        const el = document.querySelector('nav[aria-label=Projects]');
        return el.scrollTop + el.clientHeight >= el.scrollHeight - 1;
      });
      assert.ok(await isInside(last), 'wheel reveals the last project');
      await page.mouse.move(400, 10);
      await last.hover();
      if (!expanded) {
        await page.getByRole('tooltip', { name: 'Project 40', exact: true }).waitFor();
        assert.equal(Math.round((await last.boundingBox()).width), 40, 'rail target width is preserved');
        await page.mouse.move(400, 10);
        await first.focus();
        await page.getByRole('tooltip', { name: 'Project 1', exact: true }).waitFor();
        await last.focus();
      }
      await last.click();
      await page.waitForURL('**/projects/40');
      await page.reload();
      await last.waitFor({ state: 'attached' });
      await page.waitForFunction(() => document.querySelector('nav[aria-label=Projects]').scrollTop > 0);
      assert.ok(await isInside(last), 'active project is revealed after reload');
      await page.getByRole('button', { name: 'Toggle sidebar', exact: true }).click();
      assert.ok(await isInside(last), 'active project stays visible after toggling');
      // Tab must reveal the next project without requiring pointer scrolling.
      await first.focus();
      assert.ok(await isInside(first), 'focus scrolls the first project into view');
      for (let index = 0; index < 90; index++) {
        if (await last.evaluate(el => el === document.activeElement)) break;
        await page.keyboard.press('Tab');
      }
      assert.ok(await last.evaluate(el => el === document.activeElement), 'last project is reachable with Tab');
      assert.ok(await isInside(last), 'keyboard focus stays visible');
      await checkFooter();
    }

    // Pinning uses the expanded menu and persists across reloads and modes.
    await page.evaluate(() => localStorage.setItem('kamacu.sidebar', 'true'));
    await page.goto(process.argv[2] + '/settings');
    await last.waitFor({ state: 'attached' });
    assert.equal(await nav.getByRole('separator').count(), 0, 'other workspaces do not affect grouping');
    const projectMenu = name => nav.getByRole('button', { name: `Project menu: ${name}`, exact: true });
    async function pinAction(name, action) {
      const trigger = projectMenu(name);
      await trigger.focus();
      await trigger.click();
      await page.getByRole('menuitem', { name: action, exact: true }).click();
      await page.getByRole('menu').waitFor({ state: 'hidden' });
    }
    // Closing and reopening a menu must retain its pending mutation and errors.
    holdPin = true;
    failPin = true;
    await projectMenu('Project 3').focus();
    await projectMenu('Project 3').click();
    await page.getByRole('menuitem', { name: 'Pin project', exact: true }).click();
    await page.keyboard.press('Escape');
    await page.getByRole('menu').waitFor({ state: 'hidden' });
    await projectMenu('Project 3').click();
    assert.equal(await page.getByRole('menuitem', { name: 'Pin project', exact: true }).getAttribute('aria-disabled'), 'true', 'reopening keeps the pending action disabled');
    assert.equal(pinRequests, 1, 'only one pin request is in flight');
    assert.equal(typeof releasePin, 'function');
    holdPin = false;
    releasePin();
    await page.getByRole('alert').filter({ hasText: 'Could not pin project' }).waitFor();
    failPin = false;
    await page.getByRole('menuitem', { name: 'Pin project', exact: true }).click();
    await page.getByRole('menu').waitFor({ state: 'hidden' });
    assert.equal(await nav.getByRole('link').first().getAttribute('aria-label'), 'Project 3');
    await pinAction('Project 3', 'Unpin project');

    await pinAction('Project 40', 'Pin project');
    assert.equal(await nav.getByRole('link').first().getAttribute('aria-label'), 'Project 40');
    await nav.getByRole('separator', { name: 'Other projects' }).waitFor();
    await pinAction('Project 2', 'Pin project');
    assert.deepEqual(await nav.getByRole('link').evaluateAll(links => links.slice(0, 2).map(el => el.getAttribute('aria-label'))), ['Project 2', 'Project 40']);
    assert.equal(await nav.getByRole('separator').evaluate(el => [...el.parentElement.children].indexOf(el)), 2, 'separator follows all pinned projects');
    await page.reload();
    await nav.getByRole('separator').waitFor();
    assert.equal(await nav.getByRole('link').first().getAttribute('aria-label'), 'Project 2', 'pins survive reload');
    await page.getByRole('button', { name: 'Toggle sidebar', exact: true }).click();
    assert.deepEqual(await nav.getByRole('link').evaluateAll(links => links.slice(0, 2).map(el => el.getAttribute('aria-label'))), ['Project 2', 'Project 40']);
    assert.equal(await nav.getByRole('button', { name: /^Project menu:/ }).count(), 0, 'rail keeps menus hidden');
    await page.getByRole('button', { name: 'Toggle sidebar', exact: true }).click();
    failPin = true;
    await projectMenu('Project 2').click();
    await page.getByRole('menuitem', { name: 'Unpin project', exact: true }).click();
    await page.getByRole('alert').filter({ hasText: 'Could not unpin project' }).waitFor();
    assert.equal(await page.locator('nav[aria-label=Projects] a').first().getAttribute('aria-label'), 'Project 2', 'failed save preserves ordering');
    failPin = false;
    await page.getByRole('menuitem', { name: 'Unpin project', exact: true }).click();
    await page.getByRole('menu').waitFor({ state: 'hidden' });
    await pinAction('Project 40', 'Unpin project');
    assert.equal(await nav.getByRole('separator').count(), 0, 'separator disappears without pins');
    assert.equal(await nav.getByRole('link').first().getAttribute('aria-label'), 'Project 1');

    await page.setViewportSize({ width: 390, height: 600 });
    await page.goto(process.argv[2] + '/settings');
    await page.getByRole('button', { name: 'Open projects and navigation' }).click();
    await nav.waitFor();
    await checkOverflow();
    // Exercise a real browser touch gesture, rather than programmatic scrolling.
    const touch = await page.context().newCDPSession(page);
    await touch.send('Emulation.setTouchEmulationEnabled', { enabled: true, maxTouchPoints: 1 });
    await nav.hover(); // Wait for the mobile sheet's opening animation to settle.
    const touchBounds = await nav.boundingBox();
    const touchX = touchBounds.x + touchBounds.width / 2;
    const touchStartY = touchBounds.y + touchBounds.height * 0.8;
    const touchDistance = touchBounds.height * 0.6;
    await touch.send('Input.dispatchTouchEvent', {
      type: 'touchStart', touchPoints: [{ x: touchX, y: touchStartY }],
    });
    for (let step = 1; step <= 8; step++) {
      await touch.send('Input.dispatchTouchEvent', {
        type: 'touchMove', touchPoints: [{ x: touchX, y: touchStartY - touchDistance * step / 8 }],
      });
    }
    await touch.send('Input.dispatchTouchEvent', { type: 'touchEnd', touchPoints: [] });
    await page.waitForFunction(() => document.querySelector('nav[aria-label=Projects]').scrollTop > 0);
    assert.ok(await viewport.evaluate(el => el.scrollTop > 0), 'touch gesture scrolls the project list');
    await touch.send('Emulation.setTouchEmulationEnabled', { enabled: false });
    await touch.detach();
    await last.focus();
    assert.ok(await isInside(last), 'mobile drawer can reveal the last project');
    assert.equal(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth), true);

    await page.goto(process.argv[2] + '/projects/40');
    await page.getByRole('button', { name: 'Open projects and navigation' }).click();
    await last.waitFor({ state: 'visible' });
    assert.ok(await isInside(last), 'opening mobile drawer reveals the active project');

    // A long notification error must not crowd projects out of a short drawer.
    notificationStatus = { pending_count: 1, processing: { status: 'failed', error: 'Processing failed. '.repeat(100) } };
    await page.setViewportSize({ width: 600, height: 320 });
    await page.reload();
    await page.getByRole('button', { name: 'Open projects and navigation' }).click();
    await last.waitFor({ state: 'attached' });
    assert.ok((await viewport.boundingBox()).height >= 40, 'short mobile drawer retains room for projects');
    const visibleScrollbars = await page.getByRole('dialog').evaluate(dialog =>
      [...dialog.querySelectorAll('*')].filter(el => {
        const style = getComputedStyle(el);
        return el.getClientRects().length && el.scrollHeight > el.clientHeight + 1 &&
          ['auto', 'scroll'].includes(style.overflowY) && style.scrollbarWidth !== 'none' &&
          getComputedStyle(el, '::-webkit-scrollbar').display !== 'none';
      }).map(el => ({ tag: el.tagName, classes: el.className })),
    );
    assert.deepEqual(visibleScrollbars, [], 'all scrollable sections of the project sidebar hide their scrollbar');
    await last.focus();
    assert.ok(await isInside(last), 'last project remains reachable with long notifications');

    projectCount = 2;
    projects[0].pinned = true;
    projects[1].pinned = true;
    await page.setViewportSize({ width: 1280, height: 600 });
    await page.goto(process.argv[2] + '/settings');
    await nav.getByRole('link', { name: 'Project 2', exact: true }).waitFor();
    assert.equal(await nav.getByRole('separator').count(), 0, 'all-pinned lists need no separator');
    assert.equal(await viewport.evaluate(el => el.scrollHeight > el.clientHeight), false, 'short lists do not overflow');
    assert.equal(await nav.locator('[data-orientation=vertical]').count(), 0, 'short lists need no scrollbar');
    assert.deepEqual(errors, []);
    console.log('Project sidebar: wheel, click, keyboard, active-project reveal, fixed footer, rail targets, mobile, pin/unpin, grouping, persistence, save failures, and short lists passed.');
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exit(1); });
