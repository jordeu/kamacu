import { useEffect, useState } from "react";
import { Link } from "react-router";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useGithubIssues, useImportGithubIssue } from "@/api/issues";
import type { GithubIssue } from "@/api/issues";

// Mounted only while open: search, selection, errors, and mutations start fresh.
export function ImportGithubIssueDialog({
  projectId,
  repo,
  onClose,
  restoreFocus,
}: {
  projectId: number;
  repo: string;
  onClose: () => void;
  restoreFocus: () => void;
}) {
  const [search, setSearch] = useState("");
  const [query, setQuery] = useState("");
  const [assignedToMe, setAssignedToMe] = useState(true);
  const [includeClosed, setIncludeClosed] = useState(false);
  const [selected, setSelected] = useState<GithubIssue | null>(null);
  const [existingTaskId, setExistingTaskId] = useState<number | null>(null);
  const issues = useGithubIssues(projectId, query, includeClosed, assignedToMe);
  const importIssue = useImportGithubIssue(projectId);
  const waitingForSearch = search.trim() !== query;
  useEffect(() => {
    const timer = window.setTimeout(() => setQuery(search.trim()), 300);
    return () => window.clearTimeout(timer);
  }, [search]);
  const results = [
    ...new Map(
      issues.data?.pages
        .flatMap((page) => page.issues)
        .map((issue) => [issue.number, issue]),
    ).values(),
  ];
  const taskId = existingTaskId ?? selected?.task_id;
  function clearSelection() {
    setSelected(null);
    setExistingTaskId(null);
    importIssue.reset();
  }
  function submit() {
    if (!selected || importIssue.isPending || taskId || waitingForSearch)
      return;
    importIssue.mutate(selected.number, {
      onSuccess: (result) => {
        if (result.already_imported) setExistingTaskId(result.task.id);
        else onClose();
      },
    });
  }
  return (
    <Dialog
      open
      onOpenChange={(open) => {
        if (!open && !importIssue.isPending) onClose();
      }}
    >
      <DialogContent
        className="flex max-h-[85vh] flex-col sm:max-w-[680px]"
        showCloseButton={!importIssue.isPending}
        onCloseAutoFocus={(event) => {
          event.preventDefault();
          restoreFocus();
        }}
      >
        <DialogHeader>
          <DialogTitle>Import from GitHub</DialogTitle>
          <DialogDescription>
            Choose one issue from {repo} to create a task.
          </DialogDescription>
        </DialogHeader>
        <Input
          autoFocus
          aria-label="Search GitHub issues"
          placeholder="Search issues or enter #123"
          maxLength={256}
          value={search}
          disabled={importIssue.isPending}
          onChange={(event) => {
            setSearch(event.target.value);
            clearSelection();
          }}
        />
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={includeClosed}
            disabled={importIssue.isPending}
            onChange={(event) => {
              setIncludeClosed(event.target.checked);
              clearSelection();
            }}
          />
          Include closed issues
        </label>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            checked={assignedToMe}
            disabled={importIssue.isPending}
            onChange={(event) => {
              setAssignedToMe(event.target.checked);
              clearSelection();
            }}
          />
          Assigned to me
        </label>
        <div className="min-h-0 flex-1 space-y-3 overflow-y-auto">
          {issues.isPending || waitingForSearch ? (
            <p role="status" className="text-sm text-muted-foreground">
              Searching issues…
            </p>
          ) : (
            <>
              {issues.isError && (
                <div role="alert" className="space-y-2 text-sm">
                  <p>{issues.error.message}</p>
                  <Button
                    variant="outline"
                    onClick={() =>
                      issues.isFetchNextPageError
                        ? issues.fetchNextPage()
                        : issues.refetch()
                    }
                  >
                    Retry
                  </Button>
                </div>
              )}
              {!issues.isError && results.length === 0 && (
                <p role="status" className="text-sm text-muted-foreground">
                  No issues found. Try another search or adjust the filters.
                </p>
              )}
              <div
                role="radiogroup"
                aria-label="GitHub issues"
                className="space-y-1"
              >
                {results.map((issue) => (
                  <label
                    key={issue.number}
                    className={`flex cursor-pointer items-start gap-3 rounded-md border p-3 ${selected?.number === issue.number ? "border-primary bg-accent" : "border-border"}`}
                  >
                    <input
                      type="radio"
                      name="github-issue"
                      className="mt-1"
                      checked={selected?.number === issue.number}
                      disabled={importIssue.isPending}
                      onChange={() => {
                        clearSelection();
                        setSelected(issue);
                      }}
                    />
                    <span className="min-w-0 text-sm">
                      <span className="block break-words font-medium">
                        #{issue.number} {issue.title}
                      </span>
                      <span className="text-xs text-muted-foreground">
                        {issue.state} · {issue.author || "Unknown author"}
                        {issue.task_id ? " · Already imported" : ""}
                      </span>
                    </span>
                  </label>
                ))}
              </div>
              {issues.hasNextPage && (
                <Button
                  variant="outline"
                  disabled={issues.isFetchingNextPage || importIssue.isPending}
                  onClick={() => issues.fetchNextPage()}
                >
                  {issues.isFetchingNextPage ? "Loading…" : "Load more"}
                </Button>
              )}
              {issues.data && issues.data.pages.length >= 33 && (
                <p className="text-sm text-muted-foreground">
                  Search more specifically to find additional issues.
                </p>
              )}
            </>
          )}
          {selected && (
            <section
              aria-label="Selected issue preview"
              className="space-y-2 rounded-md border p-3"
            >
              <a
                href={selected.url}
                target="_blank"
                rel="noreferrer"
                className="text-sm underline"
              >
                View #{selected.number} on GitHub
              </a>
              <p className="max-h-48 overflow-y-auto whitespace-pre-wrap break-words text-sm">
                {selected.body || "No description provided."}
              </p>
            </section>
          )}
        </div>
        {taskId && (
          <p role="status" className="text-sm">
            Already imported.{" "}
            <Link
              onClick={onClose}
              className="underline"
              to={`/projects/${projectId}/tasks/${taskId}`}
            >
              Open existing task
            </Link>
          </p>
        )}
        {importIssue.isError && (
          <p role="alert" className="text-sm text-destructive">
            {importIssue.error.message}
          </p>
        )}
        <DialogFooter>
          <Button
            variant="ghost"
            disabled={importIssue.isPending}
            onClick={onClose}
          >
            Cancel
          </Button>
          <Button
            disabled={
              !selected || !!taskId || importIssue.isPending || waitingForSearch
            }
            onClick={submit}
          >
            {importIssue.isPending ? "Importing…" : "Import task"}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
