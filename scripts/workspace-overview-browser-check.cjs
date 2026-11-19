// Run against `npm run preview`; every API request is intercepted, including
// review creation. No real workspace or GitHub data is changed.
const { chromium } = require(
  process.env.KAMACU_PLAYWRIGHT_PACKAGE || "playwright",
);
const assert = require("node:assert/strict");
const { expect } = require(
  (process.env.KAMACU_PLAYWRIGHT_PACKAGE || "playwright") + "/test",
);

(async () => {
  const browser = await chromium.launch({
    headless: true,
    executablePath: process.env.KAMACU_BROWSER_EXECUTABLE,
    args: ["--no-sandbox"],
  });
  try {
    const page = await browser.newPage({
      viewport: { width: 1440, height: 1000 },
    });
    const errors = [];
    page.on("pageerror", (error) => errors.push(error.message));
    let github = true;
    let failTasks = false;
    let failReviews = false;
    let pendingReviews = false;
    let refreshRequests = 0;
    let prRequests = 0;
    let openRequests = 0;
    let activeStatusReads = 0;
    let forcedStatusReads = 0;
    let taskMoveRequests = 0;
    const projects = [
      { id: 1, name: "sched", github_repo: "org/sched", workspace_id: 1 },
      { id: 2, name: "fusion", github_repo: "org/fusion", workspace_id: 1 },
      { id: 3, name: "Local project", github_repo: null, workspace_id: 1 },
      { id: 4, name: "Other project", github_repo: null, workspace_id: 2 },
    ].map((p) => ({
      ...p,
      repo_path: "/tmp/project",
      description: "",
      managed: false,
      pinned: false,
      icon_letters: p.name.slice(0, 2).toUpperCase(),
      icon_color: "#84689e",
      agent_id: 1,
    }));
    const makeTask = (id, project_id, status) => ({
      id,
      project_id,
      status,
      source: "manual",
      title: `Restore task ${id}: ${"cross-project work ".repeat(4)}`,
      description: "",
      updated_at: "2026-11-19T07:00:00Z",
      created_at: "2026-11-19T07:00:00Z",
      position: 0,
      recurrent: false,
      branch: null,
      worktree_path: null,
      worktree_error: null,
      pr_number: null,
      pr_base_ref: null,
    });
    const tasks = [
      makeTask(101, 1, "in_progress"),
      ...Array.from({ length: 18 }, (_, i) =>
        makeTask(201 + i, 2, "in_review"),
      ),
      makeTask(301, 4, "in_progress"),
    ];
    tasks[0].pr_links = [
      {
        task_id: 101,
        repository: "org/sched",
        number: 55,
        role: "implementation",
        url: "https://github.com/org/sched/pull/55",
        linked_at: "2026-11-19T07:00:00Z",
      },
      {
        task_id: 101,
        repository: "org/other",
        number: 42,
        role: "implementation",
        url: "https://github.com/org/other/pull/42",
        linked_at: "2026-11-18T07:00:00Z",
      },
    ];
    const status = (number, repository) => ({
      repository,
      number,
      lifecycle: "open",
      head: "abcdef",
      author: "alice",
      humans: { state: "approved", reviewers: [] },
      bots: { state: "none", reviewers: [] },
      ci: { state: forcedStatusReads > 0 ? "fail" : "pass", jobs: [] },
      fetched_at: "2026-11-19T07:00:00Z",
      stale: false,
    });
    const pr = (id) => ({
      number: 17,
      title: `Review request for ${id === 1 ? "sched" : "fusion"}`,
      author: "alice",
      updatedAt: "2026-11-19T06:00:00Z",
      checks: "pass",
      url: `https://github.com/org/${id === 1 ? "sched" : "fusion"}/pull/17`,
    });
    await page.route("**/api/**", async (route) => {
      const url = new URL(route.request().url());
      const path = url.pathname;
      let data = [];
      if (path === "/api/sessions/events")
        return route.fulfill({
          contentType: "text/event-stream",
          body: ": ready\n\n",
        });
      if (path === "/api/projects") data = projects;
      else if (path === "/api/workspaces")
        data = [
          { id: 1, name: "Seqera", is_default: true },
          { id: 2, name: "Other", is_default: false },
          { id: 3, name: "Empty", is_default: false },
        ];
      else if (path === "/api/tasks") {
        assert.equal(
          url.searchParams.getAll("status").join(","),
          "in_progress,in_review",
        );
        if (failTasks)
          return route.fulfill({
            status: 500,
            json: { error: "Task fetch failed" },
          });
        const workspace = Number(url.searchParams.get("workspace_id"));
        data = tasks.filter((t) =>
          projects.some(
            (p) => p.id === t.project_id && p.workspace_id === workspace,
          ),
        );
      } else if (/\/pull-requests$/.test(path)) {
        prRequests++;
        if (url.searchParams.get("refresh") === "1") refreshRequests++;
        const id = Number(path.split("/")[3]);
        assert([1, 2].includes(id), "unlinked projects must not fetch reviews");
        if (failReviews)
          return route.fulfill({
            status: 500,
            json: { error: "GitHub unavailable" },
          });
        if (pendingReviews)
          return route.fulfill({
            json: {
              state: "ok",
              stale: false,
              fetchedAt: null,
              prs: null,
              reviewed: null,
            },
          });
        data = {
          state: "ok",
          stale: false,
          fetchedAt: "2026-11-19T07:00:00Z",
          prs: [pr(id)],
          reviewed: [{ ...pr(id), number: 99 }],
        };
      } else if (/\/pull-requests\/17\/review$/.test(path)) {
        openRequests++;
        const id = Number(path.split("/")[3]);
        data = {
          task: {
            ...makeTask(900, id, "todo"),
            source: "github_pr",
            pr_number: 17,
          },
          pr: { ...pr(id), state: "OPEN", body: "" },
        };
      } else if (/^\/api\/tasks\/\d+$/.test(path))
        data = tasks.find((t) => t.id === Number(path.split("/")[3])) || {
          ...makeTask(900, 1, "todo"),
          source: "github_pr",
          pr_number: 17,
        };
      else if (/^\/api\/projects\/\d+\/tasks$/.test(path))
        data = tasks.filter((t) => t.project_id === Number(path.split("/")[3]));
      else if (/\/task-pr-statuses$/.test(path)) {
        if (url.searchParams.get("active") === "1") activeStatusReads++;
        if (url.searchParams.get("refresh") === "1") forcedStatusReads++;
        data = { statuses: [status(55, "org/sched"), status(42, "org/other")] };
      } else if (/\/task-pr-statuses\/refresh$/.test(path)) {
        const request = route.request().postDataJSON();
        data = status(request.number, request.repository);
      } else if (/^\/api\/tasks\/\d+\/move$/.test(path)) {
        taskMoveRequests++;
        const task = tasks.find((t) => t.id === Number(path.split("/")[3]));
        Object.assign(task, { status: route.request().postDataJSON().status });
        data = task;
      } else if (path === "/api/activity") {
        const time = { n: 0, min: 0, max: 0, median: 0 };
        data = {
          tasks: [],
          reviews: { state: "ok", stale: false, fetchedAt: null, prs: [] },
          stats: {
            taskCount: 0,
            reviewCount: 0,
            cycle: time,
            dwellInProgress: time,
            dwellInReview: time,
          },
        };
      } else if (path === "/api/settings")
        data = { github_integration: { value: github ? "on" : "off" } };
      else if (path === "/api/inbox")
        data = { items: [], pending_count: 0, has_more: false };
      else if (path === "/api/notifications/status")
        data = { pending_count: 0, processing: null };
      else if (path === "/api/notifications/rules") data = [];
      else if (path === "/api/agents")
        data = [{ id: 1, engine: "custom", name: "Test" }];
      else if (path === "/api/global")
        data = {
          root_path: "",
          root_exists: false,
          live: { agent: 0, bash: 0, tmux: 0 },
          agent: { id: 1, engine: "custom" },
        };
      else if (/\/pr-statuses$/.test(path)) data = { statuses: [] };
      await route.fulfill({ json: data });
    });
    const base = process.argv[2];
    const overview = page.getByTestId("workspace-overview");
    const section = (kind) =>
      overview.locator(`section[aria-labelledby="overview-${kind}-heading"]`);
    const lane = (kind) => section(kind).locator('[tabindex="0"]').first();
    const taskCard = (task) =>
      section(task.status === "in_progress" ? "progress" : "review").getByRole(
        "button",
        {
          name: `${task.title} · ${projects.find((p) => p.id === task.project_id).name}`,
          exact: true,
        },
      );
    await page.goto(`${base}/workspaces/1/overview`);
    await expect(
      section("prs").getByRole("button", {
        name: /^Open review for (sched|fusion) PR #17$/,
      }),
    ).toHaveCount(2);
    assert.deepEqual(await overview.locator("section h2").allTextContents(), [
      "Tasks in progress",
      "Tasks in review",
      "PRs awaiting your review",
    ]);
    await expect(
      section("review").getByRole("button", { name: /^Restore task/ }),
    ).toHaveCount(18);
    assert.equal(
      await overview.getByRole("button", { name: /Show all/ }).count(),
      0,
    );
    assert.equal(
      await page.evaluate(() => localStorage.getItem("kamacu.workspace")),
      "1",
    );
    await expect.poll(() => activeStatusReads).toBeGreaterThan(0);
    const progressCard = taskCard(tasks[0]);
    await progressCard
      .getByRole("link", { name: /org\/sched PR #55/ })
      .waitFor();
    await progressCard
      .getByRole("button", { name: "Show 1 more linked pull request" })
      .click();
    await progressCard
      .getByRole("link", { name: /org\/other PR #42/ })
      .waitFor();
    assert(
      new URL(page.url()).pathname.endsWith("/overview"),
      "PR controls must not open the task",
    );
    await progressCard.locator('button[aria-label*="status"]').first().click();
    await page
      .getByRole("heading", { name: "Human reviews", exact: true })
      .waitFor();
    await page
      .getByRole("button", { name: "Refresh PR status", exact: true })
      .click();
    await page.keyboard.press("Escape");
    await page.screenshot({
      path: "/tmp/workspace-board-implemented.png",
      fullPage: true,
    });
    const footer = page.locator('[data-sidebar="footer"]:visible');
    await footer
      .getByRole("link", { name: "Workspace overview", exact: true })
      .waitFor();
    assert.equal(
      await footer.getByRole("link", { name: "Activity", exact: true }).count(),
      0,
    );
    await overview
      .getByRole("button", { name: "Refresh workspace overview" })
      .click();
    await expect.poll(() => refreshRequests).toBe(2);
    await expect.poll(() => forcedStatusReads).toBe(1);
    await expect(
      progressCard.getByRole("button", {
        name: /CI failing.*Show PR status overview/,
      }),
    ).toHaveCount(2);
    await expect(
      overview.getByRole("button", { name: "Refresh workspace overview" }),
    ).toBeEnabled();
    await section("prs")
      .getByRole("button", { name: "Filter to sched", exact: true })
      .focus();
    await page.keyboard.press("Enter");
    await expect(overview.getByLabel("Filter project")).toHaveValue("1");
    assert.equal(
      openRequests,
      0,
      "keyboard badge activation must not open a review",
    );
    await expect(
      section("review").getByRole("button", { name: /^Restore task/ }),
    ).toHaveCount(0);
    await overview.getByLabel("Search work").fill("42");
    await expect(
      section("progress").getByRole("button", {
        name: `${tasks[0].title} · sched`,
        exact: true,
      }),
    ).toHaveCount(1);
    await overview.getByLabel("Search work").fill("");
    await overview.getByLabel("Filter project").selectOption("all");
    await expect(
      section("review").getByRole("button", { name: /^Restore task/ }),
    ).toHaveCount(18);
    await lane("review").evaluate((el) => {
      el.scrollTop = 400;
    });
    await expect
      .poll(() => lane("review").evaluate((el) => el.scrollTop))
      .toBe(400);
    const toOpen = taskCard(tasks[10]);
    await toOpen.scrollIntoViewIfNeeded();
    const beforeTask = await lane("review").evaluate((el) => el.scrollTop);
    await toOpen.click();
    await page.waitForURL(`**/projects/2/tasks/${tasks[10].id}`);
    await page.goBack();
    await expect
      .poll(() => lane("review").evaluate((el) => el.scrollTop))
      .toBe(beforeTask);
    await progressCard.focus();
    await page.keyboard.press("Enter");
    await page.waitForURL("**/projects/1/tasks/101");
    await page.goBack();
    await section("prs")
      .getByRole("button", { name: /^Open review for (sched|fusion) PR #17$/ })
      .first()
      .click();
    await page.waitForURL("**/projects/1/tasks/900");
    assert.equal(openRequests, 1);
    await page.goBack();
    await page
      .getByRole("button", { name: "Workspace: Seqera", exact: true })
      .click();
    await page
      .getByRole("menuitemcheckbox", { name: "Other", exact: true })
      .click();
    await page.waitForURL("**/workspaces/2/overview");
    await expect(
      section("progress").getByRole("button", {
        name: `${tasks[19].title} · Other project`,
        exact: true,
      }),
    ).toHaveCount(1);
    await page
      .getByRole("button", { name: "Workspace: Other", exact: true })
      .click();
    await page
      .getByRole("menuitemcheckbox", { name: "Empty", exact: true })
      .click();
    await page.waitForURL("**/workspaces/3/overview");
    await overview.getByText(/No projects in this workspace/).waitFor();
    await page.goto(`${base}/workspaces/1/overview`);
    await page
      .getByRole("button", { name: "Toggle sidebar", exact: true })
      .click();
    const entry = page.getByRole("link", {
      name: "Workspace overview",
      exact: true,
    });
    await expect
      .poll(async () => Math.round((await entry.boundingBox()).width))
      .toBe(40);
    assert.equal(await entry.getAttribute("aria-current"), "page");
    const activity = page.getByRole("link", { name: "Activity", exact: true });
    assert.equal(
      await activity.evaluate((el) =>
        el.previousElementSibling?.getAttribute("title"),
      ),
      "Scratchpad",
    );
    const bar = page.getByRole("button", { name: /^Active agent sessions:/ });
    const barExpanded = await bar.getAttribute("aria-expanded");
    await activity.click();
    await page.waitForURL("**/activity");
    await expect(activity).toHaveAttribute("aria-current", "page");
    assert.equal(
      await bar.getAttribute("aria-expanded"),
      barExpanded,
      "Activity click must not toggle sessions",
    );
    await page.goBack();
    await page.setViewportSize({ width: 390, height: 844 });
    await page
      .getByRole("button", {
        name: "Open projects and navigation",
        exact: true,
      })
      .click();
    await page
      .getByRole("link", { name: "Workspace overview", exact: true })
      .click();
    await page.getByRole("dialog").waitFor({ state: "hidden" });
    assert.equal(
      await page.evaluate(
        () => document.documentElement.scrollWidth > innerWidth,
      ),
      false,
    );
    const board = overview.locator('[aria-label="Workspace board"]');
    assert(
      await board.evaluate((el) => el.scrollWidth > el.clientWidth),
      "mobile board supports horizontal scrolling",
    );
    await board.evaluate((el) => {
      el.scrollLeft = el.scrollWidth;
    });
    await expect(section("prs")).toBeInViewport();
    await page.screenshot({
      path: "/tmp/workspace-board-implemented-mobile.png",
      fullPage: true,
    });
    await activity.click();
    await page.waitForURL("**/activity");
    await page.goBack();
    pendingReviews = true;
    await page.reload();
    await overview
      .getByText("review requests not ready", { exact: false })
      .first()
      .waitFor();
    await expect(
      overview.locator('[aria-label="PRs awaiting your review: —"]'),
    ).toHaveCount(1);
    pendingReviews = false;
    await expect(
      section("prs").getByRole("button", {
        name: /^Open review for (sched|fusion) PR #17$/,
      }),
    ).toHaveCount(2, { timeout: 10000 });
    failReviews = true;
    await page.reload();
    await overview
      .getByText("PR count is partial.", { exact: false })
      .waitFor();
    await expect(
      overview.locator('[aria-label="PRs awaiting your review: —"]'),
    ).toHaveCount(1);
    assert.equal(
      await section("progress")
        .getByRole("button", { name: `${tasks[0].title} · sched`, exact: true })
        .count(),
      1,
    );
    github = false;
    prRequests = 0;
    await page.reload();
    await overview.getByText("GitHub integration is turned off.").waitFor();
    assert.equal(prRequests, 0);
    failTasks = true;
    await page.reload();
    await overview.getByText("Couldn't load tasks.", { exact: true }).waitFor();
    await expect(
      overview.locator('[aria-label="Tasks in progress: —"]'),
    ).toHaveCount(1);
    // The shared card refactor must preserve drag-and-drop in project boards.
    await page.setViewportSize({ width: 1440, height: 1000 });
    await page.goto(`${base}/projects/1`);
    const draggable = page.getByRole("button", {
      name: tasks[0].title,
      exact: true,
    });
    const source = await draggable.boundingBox();
    const destination = await page
      .getByText("In Review", { exact: true })
      .locator("..")
      .locator("..")
      .boundingBox();
    await page.mouse.move(source.x + source.width / 2, source.y + 15);
    await page.mouse.down();
    await page.mouse.move(source.x + source.width / 2 + 10, source.y + 15, {
      steps: 3,
    });
    await page.mouse.move(
      destination.x + destination.width / 2,
      destination.y + 65,
      { steps: 15 },
    );
    await page.mouse.up();
    await expect.poll(() => taskMoveRequests).toBe(1);
    assert.equal(tasks[0].status, "in_review");
    assert.deepEqual(errors, []);
    console.log(
      "PASS: shared task/PR cards, linked PR expansion and status refresh, badge filtering, all cards, independent scroll/back restoration, project drag-and-drop, overview footer, Activity beside Scratchpad without toggling sessions, mobile board, workspace switching and unavailable data.",
    );
  } finally {
    await browser.close();
  }
})().catch((error) => {
  console.error(error);
  process.exit(1);
});
