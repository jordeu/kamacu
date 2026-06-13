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
}
