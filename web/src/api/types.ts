export type Status = "todo" | "in_progress" | "in_review" | "done";

export const STATUSES: Status[] = ["todo", "in_progress", "in_review", "done"];

export const STATUS_LABELS: Record<Status, string> = {
  todo: "To Do",
  in_progress: "In Progress",
  in_review: "In Review",
  done: "Done",
};

export interface Project {
  id: number;
  name: string;
  repo_path: string;
  description: string;
  github_repo: string | null;
  // Managed marker (v1.4): true when Kamacu cloned and owns the checkout under
  // ~/.kangent/repos/ (gated-remove on delete); false for user-pointed folder
  // projects. The backend serializes this as json:"managed".
  managed: boolean;
  // v1.7 icon identity (Phase 18): two uppercase letters + a curated-palette
  // hex (#rrggbb). Both NOT NULL on the wire (never null). The backend
  // serializes these as json:"icon_letters" / json:"icon_color".
  icon_letters: string;
  icon_color: string;
  // v1.9 workspace FK (Phase 25, migration 00012): the workspace this project
  // belongs to. NOT NULL on the backend (DEFAULT 1 → Personal), so it is always
  // present on the wire, never null. The backend serializes it as
  // json:"workspace_id" between icon_color and the timestamps.
  workspace_id: number;
  // M001 (migration 00013): the agent this project runs. NOT NULL on the
  // backend (DEFAULT 1 -> the Claude seed), always present on the wire.
  agent_id: number;
  created_at: string;
  updated_at: string;
}

// v1.9 workspaces (Phase 25, migration 00012): a named grouping of projects.
// Mirrors the backend Workspace struct. `is_default` marks the protected
// "Personal" workspace (renamable but never deletable, and the fallback target
// for a stale saved active-workspace id) — key off this flag, never the name.
export interface Workspace {
  id: number;
  name: string;
  is_default: boolean;
  created_at: string;
  updated_at: string;
}

// M001 (migration 00013): a configurable agent. The Claude Code seed is
// is_system + is_default + engine="claude"; user-added agents are engine="custom".
// Mirrors the backend api.Agent struct. `is_default` marks the protected default
// (key off this flag, never the name); `is_system` marks the non-deletable seed.
export interface Agent {
  id: number;
  name: string;
  // The command template; {{worktree}} and {{session_id}} placeholders are
  // substituted at spawn. For engine="claude" this is the binary path (the
  // hook/resume argv is built internally, not from this field).
  command: string;
  engine: "claude" | "custom";
  // M001 gate follow-up: claude-engine only. Extra argv flags appended after
  // the fixed claude flags at spawn (e.g. --dangerously-skip-permissions).
  // Relocated from the global agent_extra_params setting. "" = none.
  extra_params: string;
  is_default: boolean;
  is_system: boolean;
  created_at: string;
  updated_at: string;
}

export interface Task {
  id: number;
  project_id: number;
  title: string;
  description: string;
  status: Status;
  position: number;
  created_at: string;
  updated_at: string;
  // Worktree presence (Phase 3): path set -> active; error set -> failed
  // (Retry); both null -> absent (Create worktree). Derived, no status enum.
  branch: string | null;
  worktree_path: string | null;
  worktree_error: string | null;
  // PR-review discriminator (Phase 12): a 'github_pr' task is a PR review
  // workspace, never a board card. pr_number/pr_base_ref are set only when
  // source === 'github_pr' (the backend returns them — 12-02).
  source: "manual" | "github_pr";
  pr_number: number | null;
  pr_base_ref: string | null;
}

// ---------------------------------------------------------------------------
// Phase 11 — Activity wire types (mirror the Phase 10 Go structs field-for-field).
// Source: internal/api/activity.go:22-36 + activity_helpers.go:11-34 +
// internal/github/prlist.go:324-332. The whole response is ALWAYS HTTP 200;
// degradation rides in reviews.state (Phase 10 D-02).
// ---------------------------------------------------------------------------

/**
 * The reviews-done rollup state. The 6 string literals mirror
 * rollupReviewState in activity_helpers.go:129. `auth_required` IS included
 * (gh installed but not logged in) — CONTEXT D-12 omits it but the backend
 * emits it (Phase 11 RESEARCH Pitfall 2).
 */
export type ActivityReviewState =
  | "ok"
  | "partial"
  | "error"
  | "auth_required"
  | "no_gh"
  | "disabled";

/**
 * A tasks-done entry. `doneAt` is ms-ISO (the reaper format
 * 2006-01-02T15:04:05.000Z). `inProgressAt` / `inReviewAt` are `json:"-"`
 * server-side (activity_helpers.go:15-16) and are intentionally NOT shipped
 * — do NOT recompute cycle/dwell on the client (Phase 11 RESEARCH Anti-Pattern).
 */
export interface ActivityTask {
  id: number;
  title: string;
  doneAt: string;
  projectId: number;
  projectName: string;
}

/**
 * A reviews-done entry. `completedAt` is second-precision ISO (gh closedAt,
 * set for BOTH merged and closed PRs). `url` is the https GitHub PR URL —
 * the CONTEXT D-14 click target. Provenance fields (projectName/projectId/
 * repo) are annotated by the Phase 10 aggregation loop.
 */
export interface ReviewDoneSummary {
  number: number;
  title: string;
  completedAt: string;
  url: string;
  projectName: string;
  projectId: number;
  repo: string;
}

/**
 * A min/max/median statistic. ALL four fields are int64 SECONDS (truncated
 * server-side via int64(duration.Seconds()), activity_helpers.go:100-102) —
 * NOT milliseconds. `n` is the count of rows the stat was computed over (may
 * be less than taskCount when timestamps are missing — Phase 11 RESEARCH
 * Pitfall 3).
 */
export interface TimeStat {
  n: number;
  min: number;
  max: number;
  median: number;
}

/**
 * The stats block: counts + three time-stat rows. Reviews contribute
 * `reviewCount` ONLY (never a time-stat row — STATS-04: GitHub's merge
 * signal has no Kamacu in-progress/in-review timestamps).
 */
export interface StatsBlock {
  taskCount: number;
  reviewCount: number;
  cycle: TimeStat;
  dwellInProgress: TimeStat;
  dwellInReview: TimeStat;
}

/**
 * The combined GET /api/activity payload (Phase 10 D-01): one endpoint →
 * one TanStack query, one loading state. The reviews sub-object mirrors
 * aggregateReviewsResult (activity.go:22-27); `fetchedAt` is null until the
 * first successful gh fetch.
 */
export interface ActivityResponse {
  tasks: ActivityTask[];
  reviews: {
    state: ActivityReviewState;
    stale: boolean;
    fetchedAt: string | null;
    prs: ReviewDoneSummary[];
  };
  stats: StatsBlock;
}
