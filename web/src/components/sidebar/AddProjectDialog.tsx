import { useState, type FormEvent } from "react";
import { useNavigate } from "react-router";
import { ApiError } from "@/api/client";
import { useCreateProject } from "@/api/mutations";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";

export interface AddProjectDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/** Uppercase the first letter so server messages render in sentence case. */
function sentenceCase(message: string): string {
  return message.charAt(0).toUpperCase() + message.slice(1);
}

export function AddProjectDialog({ open, onOpenChange }: AddProjectDialogProps) {
  const [repoPath, setRepoPath] = useState("");
  const [name, setName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const createProject = useCreateProject();
  const navigate = useNavigate();

  function handleOpenChange(next: boolean) {
    if (!next) setError(null);
    onOpenChange(next);
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    try {
      const project = await createProject.mutateAsync({
        repo_path: repoPath.trim(),
        name: name.trim() || undefined,
      });
      onOpenChange(false);
      setRepoPath("");
      setName("");
      navigate(`/projects/${project.id}`);
    } catch (err) {
      if (err instanceof ApiError) {
        // Mirror the server message verbatim, sentence-cased. The duplicate
        // message ("this repository is already added") renders as a full
        // sentence per UI-SPEC: "This repository is already added."
        let message = sentenceCase(err.message);
        if (err.status === 409 && !message.endsWith(".")) message += ".";
        setError(message);
      } else {
        setError("Couldn't add the project. Try again.");
      }
      // Field values are intentionally kept so the path can be corrected.
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[480px]" aria-describedby={undefined}>
        <DialogHeader>
          <DialogTitle>Add project</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <label htmlFor="add-project-path" className="text-xs font-medium">
              Repository path
            </label>
            <Input
              id="add-project-path"
              className="font-mono"
              placeholder="/home/you/code/my-repo"
              value={repoPath}
              onChange={(event) => setRepoPath(event.target.value)}
              autoFocus
            />
            {error && <p className="text-xs text-destructive">{error}</p>}
          </div>
          <div className="flex flex-col gap-1.5">
            <label htmlFor="add-project-name" className="text-xs font-medium">
              Name
            </label>
            <Input
              id="add-project-name"
              placeholder="Defaults to folder name"
              value={name}
              onChange={(event) => setName(event.target.value)}
            />
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="ghost"
              onClick={() => handleOpenChange(false)}
            >
              Cancel
            </Button>
            <Button
              type="submit"
              disabled={createProject.isPending || repoPath.trim() === ""}
            >
              Add project
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
