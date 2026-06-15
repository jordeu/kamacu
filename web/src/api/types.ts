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
  // Managed marker (v1.4): true when Kangent cloned and owns the checkout under
  // ~/.kangent/repos/ (gated-remove on delete); false for user-pointed folder
  // projects. The backend serializes this as json:"managed".
  managed: boolean;
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
