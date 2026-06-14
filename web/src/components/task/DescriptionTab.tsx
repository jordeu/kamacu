import { useState } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import { useUpdateTask } from "@/api/mutations";
import type { Task } from "@/api/types";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";

/**
 * Read-only markdown body (a PR review — Phase 12 D-11): the same prose block,
 * NO Edit button, NO Textarea, NO Save. Review context lifted from GitHub that
 * must not drift. Split into its own component so the editable variant's hooks
 * are never conditionally skipped (rules-of-hooks).
 */
function ReadOnlyDescription({ source }: { source: string }) {
  if (!source) {
    return <span className="text-muted-foreground">No description.</span>;
  }
  return (
    <div className="prose prose-invert prose-sm max-w-none prose-a:text-blue-500">
      <ReactMarkdown remarkPlugins={[remarkGfm]}>{source}</ReactMarkdown>
    </div>
  );
}

/**
 * Markdown description with an explicit edit/preview toggle (D-10):
 * no autosave, no side-by-side preview. react-markdown's defaults are the
 * security posture — raw HTML stays unrendered, javascript: URLs are
 * neutralized. Do not add any raw-HTML rehype plugin.
 *
 * When `readOnlySource` is provided (a PR review — Phase 12 D-11), the body is
 * rendered read-only via ReadOnlyDescription instead.
 */
export function DescriptionTab({
  task,
  projectId,
  readOnlySource,
}: {
  task: Task;
  projectId: number;
  readOnlySource?: string;
}) {
  if (readOnlySource !== undefined) {
    return <ReadOnlyDescription source={readOnlySource} />;
  }

  return (
    <EditableDescription task={task} projectId={projectId} />
  );
}

function EditableDescription({
  task,
  projectId,
}: {
  task: Task;
  projectId: number;
}) {
  const updateTask = useUpdateTask(projectId);
  // null = view mode; a string (possibly empty) = edit mode draft.
  const [draft, setDraft] = useState<string | null>(null);
  const [saveFailed, setSaveFailed] = useState(false);

  function startEditing() {
    setSaveFailed(false);
    setDraft(task.description);
  }

  async function save() {
    if (draft === null) return;
    try {
      await updateTask.mutateAsync({ id: task.id, description: draft });
      setDraft(null);
      setSaveFailed(false);
    } catch {
      // The draft is never cleared on failure (UI-SPEC).
      setSaveFailed(true);
    }
  }

  function cancel() {
    setDraft(null);
    setSaveFailed(false);
  }

  if (draft === null) {
    if (!task.description) {
      return (
        <div className="flex items-center gap-2">
          <span className="text-muted-foreground">No description.</span>
          <Button variant="ghost" size="sm" onClick={startEditing}>
            Edit
          </Button>
        </div>
      );
    }
    return (
      <div className="space-y-3">
        <div className="prose prose-invert prose-sm max-w-none prose-a:text-blue-500">
          <ReactMarkdown remarkPlugins={[remarkGfm]}>
            {task.description}
          </ReactMarkdown>
        </div>
        <Button variant="ghost" size="sm" onClick={startEditing}>
          Edit
        </Button>
      </div>
    );
  }

  return (
    <div className="space-y-3">
      <Textarea
        autoFocus
        value={draft}
        onChange={(e) => setDraft(e.target.value)}
        className="min-h-[240px]"
        placeholder="Add a description…"
      />
      <div className="flex items-center gap-2">
        <Button onClick={save} disabled={updateTask.isPending}>
          Save description
        </Button>
        <Button variant="ghost" onClick={cancel}>
          Cancel
        </Button>
        {saveFailed && (
          <span className="text-xs text-destructive">
            {"Couldn't save. Try again."}
          </span>
        )}
      </div>
    </div>
  );
}
