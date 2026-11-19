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
      ...Array.from({ length: 7 }, (_, i) => makeTask(201 + i, 2, "in_review")),
      makeTask(301, 4, "in_progress"),
    ];
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
      else if (path === "/api/settings")
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
    const section = (id) =>
      overview.locator(`section[aria-labelledby="overview-${id}-heading"]`);
    await page.goto(`${base}/workspaces/1/overview`);
    await overview
      .getByRole("button", { name: /Open review for org\/sched/ })
      .waitFor();
    assert.deepEqual(await overview.locator("section h2").allTextContents(), [
      "Tasks in progress",
      "Tasks in review",
      "PRs awaiting your review",
    ]);
    assert.equal(await section("progress").locator("li").count(), 1);
    assert.equal(await section("review").locator("li").count(), 5);
    assert.equal(
      await section("prs").locator("li").count(),
      2,
      "equal PR numbers from different repos are retained",
    );
    assert.equal(
      await page.evaluate(() => localStorage.getItem("kamacu.workspace")),
      "1",
      "URL selects workspace",
    );
    await page.screenshot({
      path: "/tmp/workspace-overview-implemented.png",
      fullPage: true,
    });
    await overview
      .getByRole("button", { name: "Refresh workspace overview" })
      .click();
    await expect.poll(() => refreshRequests).toBe(2);
    await expect(
      overview.getByRole("button", { name: "Refresh workspace overview" }),
    ).toBeEnabled();
    await overview.evaluate((element) => {
      element.scrollTop = 150;
    });
    const beforeExpand = await overview.evaluate(
      (element) => element.scrollTop,
    );
    assert(beforeExpand > 0, "fixture must allow scrolling before expansion");
    await section("review")
      .getByRole("button", { name: /Show all/ })
      .click();
    await expect(section("review").locator("li")).toHaveCount(7);
    await expect
      .poll(() => overview.evaluate((element) => element.scrollTop))
      .toBe(beforeExpand);
    const lastReview = section("review").locator("li").last().getByRole("link");
    await lastReview.scrollIntoViewIfNeeded();
    const beforeTask = await overview.evaluate((element) => element.scrollTop);
    await lastReview.click();
    await page.waitForURL("**/projects/2/tasks/207");
    await page.goBack();
    await expect(section("review").locator("li")).toHaveCount(7);
    await expect
      .poll(() => overview.evaluate((element) => element.scrollTop))
      .toBe(beforeTask);
    await overview.getByLabel("Filter project").selectOption("1");
    await expect(section("review").locator("li")).toHaveCount(0);
    await expect(section("prs").locator("li")).toHaveCount(1);
    await overview.getByLabel("Search work").fill("alice");
    await expect(section("progress").locator("li")).toHaveCount(0);
    await expect(section("prs").locator("li")).toHaveCount(1);
    await overview.getByLabel("Search work").fill("");
    await section("progress").getByRole("link").click();
    await page.waitForURL("**/projects/1/tasks/101");
    await page.goBack();
    await overview.waitFor();
    assert.equal(await overview.getByLabel("Filter project").inputValue(), "1");
    await overview
      .getByRole("button", { name: /Open review for org\/sched/ })
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
    await overview
      .locator("li")
      .getByText("Other project", { exact: true })
      .waitFor();
    assert.equal(await section("progress").locator("li").count(), 1);
    assert.equal(await section("prs").locator("li").count(), 0);
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
    await entry.waitFor();
    await expect
      .poll(async () => Math.round((await entry.boundingBox()).width))
      .toBe(40);
    assert.equal(await entry.getAttribute("aria-current"), "page");
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
    await page.screenshot({
      path: "/tmp/workspace-overview-implemented-mobile.png",
      fullPage: true,
    });
    pendingReviews = true;
    await page.reload();
    await overview
      .getByText("review requests not ready", { exact: false })
      .first()
      .waitFor();
    await expect(
      overview.locator('[aria-label="PRs awaiting your review: —"]'),
    ).toHaveCount(1);
    assert(!(await section("prs").innerText()).includes("No PRs are waiting"));
    pendingReviews = false;
    await expect(
      section("prs").getByRole("button", {
        name: /Open review for org\/sched/,
      }),
    ).toBeVisible({ timeout: 10000 });
    failReviews = true;
    await page.reload();
    await overview
      .getByText("PR count is partial.", { exact: false })
      .waitFor();
    assert.equal(
      await overview
        .locator('[aria-label="PRs awaiting your review: —"]')
        .count(),
      1,
    );
    assert.equal(
      await section("progress").locator("li").count(),
      1,
      "GitHub failure does not hide tasks",
    );
    github = false;
    prRequests = 0;
    await page.reload();
    await overview.getByText("GitHub integration is turned off.").waitFor();
    assert.equal(prRequests, 0);
    failTasks = true;
    await page.reload();
    await overview.getByText("Couldn't load tasks.", { exact: true }).waitFor();
    assert.equal(
      await overview.locator('[aria-label="Tasks in progress: —"]').count(),
      1,
    );
    assert.deepEqual(errors, []);
    console.log(
      "PASS: queue order, counts, expansion, filters, task/review navigation, workspace switching, empty workspace, collapsed entry, mobile navigation, GitHub failures/off, task errors.",
    );
  } finally {
    await browser.close();
  }
})().catch((error) => {
  console.error(error);
  process.exit(1);
});
