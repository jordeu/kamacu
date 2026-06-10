import { useState } from "react";
import { Input } from "@/components/ui/input";
import { useCreateTask } from "@/api/mutations";

/**
 * Inline quick-add row at the top of the To Do column (D-08).
 * Enter commits (and stays open for rapid entry), Esc cancels, empty blur collapses.
 * New tasks land at the top of To Do via the server position rule (D-09).
 */
export function QuickAdd({ projectId }: { projectId: number }) {
  const [open, setOpen] = useState(false);
  const [title, setTitle] = useState("");
  const createTask = useCreateTask(projectId);

  if (!open) {
    return (
      <button
        type="button"
        onClick={() => setOpen(true)}
        className="rounded-md px-3 py-2 text-left text-sm text-muted-foreground hover:bg-[#27272a]"
      >
        + New task
      </button>
    );
  }

  return (
    <Input
      autoFocus
      value={title}
      placeholder="Task title"
      onChange={(e) => setTitle(e.target.value)}
      onKeyDown={(e) => {
        if (e.key === "Enter") {
          const trimmed = title.trim();
          if (trimmed !== "") {
            createTask.mutate({ title: trimmed });
            setTitle("");
          }
        } else if (e.key === "Escape") {
          // Collapse without creating; board-level handlers never see it.
          e.stopPropagation();
          setTitle("");
          setOpen(false);
        }
      }}
      onBlur={() => {
        if (title.trim() === "") setOpen(false);
      }}
    />
  );
}
