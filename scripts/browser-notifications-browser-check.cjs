// Run against Vite with isolated API fixtures and a mocked OS notification surface.
const { chromium } = require(process.env.KAMACU_PLAYWRIGHT_PACKAGE || 'playwright');
const assert = require('node:assert/strict');

(async () => {
  const browser = await chromium.launch({ headless: true, executablePath: process.env.KAMACU_BROWSER_EXECUTABLE, args: ['--no-sandbox'] });
  try {
    const context = await browser.newContext();
    const page = await context.newPage();
    const errors = [];
    page.on('pageerror', error => { errors.push(error.message); console.error('Browser error:', error.message); });
    page.on('console', message => { if (message.type() === 'error') console.error('Browser console:', message.text()); });
    await context.addInitScript(() => {
      window.testNotifications = [];
      window.notificationPermission = localStorage.getItem('test:notification-permission') || 'default';
      window.permissionRequests = 0;
      window.notificationLockRequests = 0;
      const requestLock = navigator.locks.request.bind(navigator.locks);
      navigator.locks.request = (...args) => {
        if (args[0] === 'kamacu:inbox-notifications') window.notificationLockRequests++;
        return requestLock(...args);
      };
      class MockNotification {
        static get permission() { return window.notificationPermission; }
        static async requestPermission() {
          window.permissionRequests++;
          window.notificationPermission = 'granted';
          localStorage.setItem('test:notification-permission', 'granted');
          return 'granted';
        }
        constructor(title, options) {
          this.title = title;
          this.options = options;
          window.testNotifications.push(this);
        }
        close() { this.closed = true; }
      }
      window.Notification = MockNotification;
    });
    let enabled = 'on';
    const message = id => ({ id, question: `Question ${id}`, summary: `Summary ${id}`, abstract: '', context: '', status: 'pending', revision: 1, task_id: 0, session_id: 'session', options: [], created_at: Date.now(), updated_at: Date.now() });
    let messages = [message('backlog')];
    let snapshots = 0;
    let pollingPage;
    const waitUntil = async predicate => {
      const deadline = Date.now() + 15000;
      while (!predicate()) {
        assert.ok(Date.now() < deadline, 'Timed out waiting for notification polling');
        await new Promise(resolve => setTimeout(resolve, 50));
      }
    };
    const settings = () => Object.fromEntries(['worktree_base', 'shell', 'branch_template', 'done_session_ttl', 'github_integration', 'pr_review_seed', 'browser_notifications'].map(key => [key, { value: key === 'browser_notifications' ? enabled : '', default: key === 'browser_notifications' ? 'on' : '' }]));
    await context.route('**/api/**', async route => {
      const url = new URL(route.request().url());
      if (!url.pathname.startsWith('/api/')) return route.continue();
      if (url.pathname === '/api/sessions/events') return route.fulfill({ contentType: 'text/event-stream', body: ': connected\n\n' });
      let data = [];
      if (url.pathname === '/api/settings/browser_notifications') {
        enabled = route.request().postDataJSON().value;
        data = { value: enabled, default: 'on' };
      } else if (url.pathname === '/api/settings') data = settings();
      else if (url.pathname === '/api/inbox/catch-up') {
        snapshots++;
        pollingPage = route.request().frame().page();
        data = messages;
      }
      else if (url.pathname === '/api/inbox') {
        data = { items: messages, has_more: false, pending_count: messages.length };
      } else if (url.pathname.startsWith('/api/inbox/')) data = messages.find(message => url.pathname.endsWith(message.id));
      else if (url.pathname === '/api/workspaces') data = [{ id: 1, name: 'Work' }];
      else if (url.pathname === '/api/notifications/status') data = { pending_count: 0, processing: null };
      else if (url.pathname === '/api/notifications/rules') data = [{ id: 'rules', revision: 1, status: 'active', rules: [] }];
      else if (url.pathname === '/api/worktrees') data = { counts: { total: 0, orphaned: 0 }, projects: [] };
      else if (url.pathname === '/api/monitor') data = { profiles: [], default_profile_id: null, workspaces: [], usage: {} };
      await route.fulfill({ json: data ?? {} });
    });
    await page.goto(`${process.argv[2]}/settings`);
    await page.getByRole('button', { name: 'Allow notifications', exact: true }).waitFor({ timeout: 10000 }).catch(async error => {
      console.error('Page:', await page.locator('body').innerText());
      console.error('Notification state:', await page.evaluate(() => ({ secure: isSecureContext, permission: Notification.permission })));
      throw error;
    });
    await page.waitForFunction(() => document.querySelector('[role=switch]'));
    assert.equal(await page.getByRole('switch', { name: 'Notify me about new inbox messages' }).getAttribute('aria-checked'), 'true');
    assert.equal(snapshots, 0, 'permission must be granted before polling');
    await page.getByRole('button', { name: 'Allow notifications', exact: true }).click();
    await waitUntil(() => snapshots > 0);
    assert.equal(await page.evaluate(() => window.permissionRequests), 1);
    assert.equal(await page.evaluate(() => window.testNotifications.length), 0);

    // Replacing the backlog preserves the pending count: identity must drive delivery.
    messages = [message('new')];
    await page.waitForFunction(() => window.testNotifications.length === 1);
    assert.equal(await page.evaluate(() => window.testNotifications[0].options.body), 'Question new');
    const lockRequests = await page.evaluate(() => window.notificationLockRequests);
    await page.evaluate(() => window.testNotifications[0].onclick());
    await page.waitForURL('**/inbox?message=new');
    assert.equal(await page.evaluate(() => window.testNotifications[0].closed), true);
    assert.equal(await page.evaluate(() => window.notificationLockRequests), lockRequests, 'route changes must preserve notification ownership and baseline');

    const beforeReload = snapshots;
    await page.goto(`${process.argv[2]}/settings`);
    await waitUntil(() => snapshots > beforeReload);
    const second = await context.newPage();
    second.on('pageerror', error => errors.push(error.message));
    await second.goto(`${process.argv[2]}/settings`);
    await second.getByRole('switch', { name: 'Notify me about new inbox messages' }).waitFor();
    // Both tabs are enabled, but only one may hold the notification poller.
    await second.waitForFunction(async () => (await navigator.locks.query()).pending.some(lock => lock.name === 'kamacu:inbox-notifications'));
    messages = [message('both-tabs')];
    await waitUntil(() => snapshots > 1);
    await page.waitForFunction(() => window.testNotifications.length === 1);
    assert.equal(await second.evaluate(() => window.testNotifications.length), 0, 'another tab must not duplicate notifications');

    await second.getByRole('switch', { name: 'Notify me about new inbox messages' }).click();
    await page.waitForFunction(() => document.querySelector('[role=switch]')?.getAttribute('aria-checked') === 'false');
    assert.equal(enabled, 'off');
    messages = [message('muted')];
    const before = snapshots;
    await new Promise(resolve => setTimeout(resolve, 3500));
    assert.equal(snapshots, before, 'disabled notifications must stop polling in all tabs');
    assert.equal(await page.evaluate(() => window.testNotifications.length), 1);
    await second.getByRole('switch', { name: 'Notify me about new inbox messages' }).click();
    await page.waitForFunction(() => document.querySelector('[role=switch]')?.getAttribute('aria-checked') === 'true');
    assert.equal(enabled, 'on');
    await waitUntil(() => snapshots > before);
    assert.equal(await page.evaluate(() => window.testNotifications.length) + await second.evaluate(() => window.testNotifications.length), 1, 'reenabling must skip muted messages');
    messages = [message('after-enable'), message('muted')];
    const owner = pollingPage;
    await owner.waitForFunction(() => window.testNotifications.some(notification => notification.options.body === 'Question after-enable'));
    assert.equal(await page.evaluate(() => window.testNotifications.length) + await second.evaluate(() => window.testNotifications.length), 2);

    // Closing the owner releases the lock, allowing the other tab to take over.
    const survivor = owner === page ? second : page;
    const survivorCount = await survivor.evaluate(() => window.testNotifications.length);
    const beforeHandoff = snapshots;
    await owner.close();
    await waitUntil(() => snapshots > beforeHandoff);
    messages = [message('after-handoff'), ...messages];
    await survivor.waitForFunction(count => window.testNotifications.length === count + 1, survivorCount);
    assert.equal(await survivor.evaluate(() => window.testNotifications.at(-1).options.body), 'Question after-handoff');
    assert.deepEqual(errors, []);
    console.log('Browser notifications: permission, backlog, click-through, cross-tab deduplication, settings sync, disabled polling, and tab handoff verified.');
  } finally {
    await browser.close();
  }
})().catch(error => { console.error(error); process.exitCode = 1; });
