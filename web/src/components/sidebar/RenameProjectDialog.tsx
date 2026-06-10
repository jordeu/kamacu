import { useEffect, useState, type FormEvent } from "react";
import { ApiError } from "@/api/client";
import { useRenameProject } from "@/api/mutations";
import type { Project } from "@/api/types";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";

export interface RenameProjectDialogProps {
  project: Project;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/** Uppercase the first letter so server messages render in sentence case. */
function sentenceCase(message: string): string {
  return message.charAt(0).toUpperCase() + message.slice(1);
}

export function RenameProjectDialog({
  project,
  open,
  onOpenChange,
}: RenameProjectDialogProps) {
  const [name, setName] = useState(project.name);
  const [error, setError] = useState<string | null>(null);
  const renameProject = useRenameProject();

  // Prefill with the current name each time the dialog opens.
  useEffect(() => {
    if (open) {
      setName(project.name);
      setError(null);
    }
  }, [open, project.name]);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    try {
      await renameProject.mutateAsync({ id: project.id, name: name.trim() });
      onOpenChange(false);
    } catch (err) {
      setError(
        err instanceof ApiError
          ? sentenceCase(err.message)
          : "Couldn't save. Try again.",
      );
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent aria-describedby={undefined}>
        <DialogHeader>
          <DialogTitle>Rename project</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <label
              htmlFor={`rename-project-${project.id}`}
              className="text-xs font-medium"
            >
              Name
            </label>
            <Input
              id={`rename-project-${project.id}`}
              value={name}
              onChange={(event) => setName(event.target.value)}
              autoFocus
            />
            {error && <p className="text-xs text-destructive">{error}</p>}
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="ghost"
              onClick={() => onOpenChange(false)}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={renameProject.isPending || name.trim() === ""}
            >
              Save
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
