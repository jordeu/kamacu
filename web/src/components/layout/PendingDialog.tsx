import { useState } from "react";
import { Link, useMatch } from "react-router";
import { CircleHelp } from "lucide-react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { get, post } from "@/api/client";
import { useProjects, useWorkspaces } from "@/api/queries";
import { useActiveWorkspace } from "@/lib/useActiveWorkspace";
import { Button } from "@/components/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";

type Scope = "project" | "workspace" | "anywhere";
type CardStatus = "in_review" | "in_progress";
type TerminalState = "closed" | "active";
interface PendingItem {
  task_id: number;
  project_id: number;
  title: string;
  project: string;
  state: string;
  detail?: string;
}
interface PendingJob { id: string; state: string; items: PendingItem[] }

export function PendingDialog() {
  const [open, setOpen] = useState(false);
  const [scope, setScope] = useState<Scope>("workspace");
  const [statuses, setStatuses] = useState<CardStatus[]>(["in_review", "in_progress"]);
  const [terminalStates, setTerminalStates] = useState<TerminalState[]>(["closed", "active"]);
  const projectMatch = useMatch("/projects/:projectId/*");
  const projectId = projectMatch?.params.projectId;
  const { activeWorkspaceId } = useActiveWorkspace();
  const { data: projects } = useProjects();
  const { data: workspaces } = useWorkspaces();
  const project = projects?.find((p) => p.id === Number(projectId));
  const workspace = workspaces?.find((w) => w.id === activeWorkspaceId);
  const client = useQueryClient();
  const filter = {
    scope,
    project_id: project?.id ?? 0,
    workspace_id: activeWorkspaceId ?? 0,
    statuses,
    terminal_states: terminalStates,
  };
  const valid = statuses.length > 0 && terminalStates.length > 0
    && (scope !== "project" || !!project) && (scope !== "workspace" || !!workspace);
  const job = useQuery({
    queryKey: ["pending-check"],
    queryFn: ({ signal }) => get<PendingJob>("/api/pending", { signal }),
    enabled: open,
    refetchInterval: open ? 2000 : false,
  });
  const running = job.data?.state === "running" || job.data?.state === "cancelling";
  const preview = useQuery({
    queryKey: ["pending-preview", filter],
    queryFn: ({ signal }) => post<{ eligible: number; skipped: number }>("/api/pending/preview", filter, { signal }),
    enabled: open && valid && !running,
    refetchInterval: open && !running ? 5000 : false,
  });
  const start = useMutation({
    mutationFn: () => post<PendingJob>("/api/pending", filter),
    onMutate: () => client.cancelQueries({ queryKey: ["pending-check"] }),
    onSuccess: (data) => client.setQueryData(["pending-check"], data),
    onSettled: () => client.invalidateQueries({ queryKey: ["pending-check"] }),
  });
  const cancel = useMutation({
    mutationFn: () => post<PendingJob>("/api/pending/cancel", { id: job.data?.id }),
    onMutate: () => client.cancelQueries({ queryKey: ["pending-check"] }),
    onSuccess: (data) => client.setQueryData(["pending-check"], data),
  });
  const items = job.data?.items ?? [];
  const finished = items.filter((item) => !["queued", "checking", "running"].includes(item.state)).length;
  const error = start.error ?? cancel.error ?? job.error ?? preview.error;
  return (
    <>
      <Button
        variant="ghost"
        size="icon-sm"
        aria-label="What's pending?"
        title="What's pending?"
        className="text-muted-foreground"
        onClick={(e) => { e.stopPropagation(); setOpen(true); }}
      >
        <CircleHelp className="size-3.5" />
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent className="sm:max-w-lg" onClick={(e) => e.stopPropagation()}>
          <DialogHeader>
            <DialogTitle>What's pending?</DialogTitle>
            <DialogDescription>Ask agents to check their task, report what is pending, and ask what to do next.</DialogDescription>
          </DialogHeader>
          <fieldset disabled={running || start.isPending} className="space-y-4 disabled:opacity-60">
            <div className="space-y-1.5">
              <label htmlFor="pending-scope" className="font-medium">Scope</label>
              <select id="pending-scope" value={scope} onChange={(e) => setScope(e.target.value as Scope)} className="w-full rounded-md border border-input bg-background px-3 py-2">
                <option value="project" disabled={!project}>This project{project ? ` · ${project.name}` : " (open a project first)"}</option>
                <option value="workspace" disabled={!workspace}>This workspace{workspace ? ` · ${workspace.name}` : ""}</option>
                <option value="anywhere">Anywhere · all workspaces</option>
              </select>
            </div>
            <fieldset className="space-y-2">
              <legend className="mb-1 font-medium">Card status</legend>
              {([['in_review', 'In Review'], ['in_progress', 'In Progress']] as const).map(([value, label]) => (
                <label key={value} className="mr-4 inline-flex items-center gap-2">
                  <input type="checkbox" checked={statuses.includes(value)} onChange={(e) => setStatuses((current) => e.target.checked ? [...current, value] : current.filter((s) => s !== value))} />
                  {label}
                </label>
              ))}
            </fieldset>
            <fieldset className="space-y-2">
              <legend className="mb-1 font-medium">Terminal state</legend>
              {([['closed', 'Closed'], ['active', 'Active']] as const).map(([value, label]) => (
                <label key={value} className="mr-4 inline-flex items-center gap-2">
                  <input type="checkbox" checked={terminalStates.includes(value)} onChange={(e) => setTerminalStates((current) => e.target.checked ? [...current, value] : current.filter((s) => s !== value))} />
                  {label}
                </label>
              ))}
            </fieldset>
          </fieldset>
          <p className="text-xs text-muted-foreground">
            Working agents and waiting agents with pending inbox messages are skipped. Closed terminals resume existing conversations and stay open. Scratchpad and snoozed cards are excluded.
          </p>
          <p className="text-xs text-muted-foreground">
            Two checks at a time, at least five seconds between starts. You can close this dialog while checks run. A server restart stops the queue.
          </p>
          {!running && valid && <p aria-live="polite" className="text-sm">{preview.isPending ? "Counting matching cards…" : preview.data ? `${preview.data.eligible} eligible cards · ${preview.data.skipped} skipped` : "Could not count matching cards"}</p>}
          {!valid && <p className="text-sm">Select a scope, at least one card status and at least one terminal state.</p>}
          {error && <p role="alert" className="text-sm text-destructive">{error.message}</p>}
          {job.data && job.data.state !== "idle" && (
            <div className="space-y-2">
              <p role="status" className="text-sm font-medium">
                {job.data.state === "cancelling" ? "Cancelling queued checks; started checks will finish" : job.data.state === "running" ? "Checking tasks" : job.data.state === "cancelled" ? "Checks cancelled" : "Checks finished"} · {finished}/{items.length}
              </p>
              <ul className="max-h-48 space-y-2 overflow-y-auto text-xs">
                {items.map((item) => <li key={item.task_id}>
                  <Link className="font-medium hover:underline" to={`/projects/${item.project_id}/tasks/${item.task_id}`} onClick={() => setOpen(false)}>{item.project} · {item.title}</Link>
                  <span className="ml-2 text-muted-foreground">{item.state}{item.detail ? ` · ${item.detail}` : ""}</span>
                </li>)}
              </ul>
            </div>
          )}
          <div className="flex justify-end gap-2">
            {running ? <Button variant="outline" disabled={cancel.isPending || job.data?.state === "cancelling"} onClick={() => cancel.mutate()}>Cancel queued checks</Button>
              : <Button disabled={!valid || job.isPending || job.isError || start.isPending || !preview.data?.eligible || preview.isError || preview.isFetching} onClick={() => { cancel.reset(); start.mutate(); }}>
                {start.isPending ? "Starting…" : "Ask what's pending"}
              </Button>}
          </div>
        </DialogContent>
      </Dialog>
    </>
  );
}
