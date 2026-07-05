import { useEffect, useState, type FormEvent } from "react";
import { ApiError } from "@/api/client";
import { useCreateWorkspace, useRenameWorkspace } from "@/api/mutations";
import type { Workspace } from "@/api/types";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";

export interface WorkspaceNameDialogProps {
  /** "create" starts empty; "rename" prefills from `workspace`. */
  mode: "create" | "rename";
  /** Required in "rename" mode — the workspace being renamed. */
  workspace?: Workspace;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/** Uppercase the first letter so server messages render in sentence case. */
function sentenceCase(message: string): string {
  return message.charAt(0).toUpperCase() + message.slice(1);
}

/**
 * Create/rename workspace dialog (D-04) — a near-verbatim clone of
 * RenameProjectDialog. One dialog serves both flows: "create" opens with an
 * empty field ("New workspace" / "Create"), "rename" prefills the existing name
 * ("Rename workspace" / "Save"). The server's duplicate-name reject
 * (`name UNIQUE COLLATE NOCASE`, migration 00012) arrives as `ApiError.message`
 * and renders sentence-cased inline; Save is disabled while pending or empty.
 */
export function WorkspaceNameDialog({
  mode,
  workspace,
  open,
  onOpenChange,
}: WorkspaceNameDialogProps) {
  const [name, setName] = useState(mode === "rename" ? (workspace?.name ?? "") : "");
  const [error, setError] = useState<string | null>(null);
  const createWorkspace = useCreateWorkspace();
  const renameWorkspace = useRenameWorkspace();

  // The active mutation drives the pending/disabled state.
  const pending =
    mode === "create" ? createWorkspace.isPending : renameWorkspace.isPending;

  // Prefill each time the dialog opens: current name for rename, empty for
  // create. Clear any prior error.
  useEffect(() => {
    if (open) {
      setName(mode === "rename" ? (workspace?.name ?? "") : "");
      setError(null);
    }
  }, [open, mode, workspace?.name]);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    try {
      if (mode === "create") {
        await createWorkspace.mutateAsync({ name: name.trim() });
      } else {
        await renameWorkspace.mutateAsync({ id: workspace!.id, name: name.trim() });
      }
      onOpenChange(false);
    } catch (err) {
      setError(
        err instanceof ApiError
          ? sentenceCase(err.message)
          : "Couldn't save. Try again.",
      );
    }
  }

  const title = mode === "create" ? "New workspace" : "Rename workspace";
  const submitLabel = mode === "create" ? "Create" : "Save";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent aria-describedby={undefined}>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <label htmlFor="workspace-name" className="text-xs font-medium">
              Name
            </label>
            <Input
              id="workspace-name"
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
            <Button type="submit" disabled={pending || name.trim() === ""}>
              {submitLabel}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
