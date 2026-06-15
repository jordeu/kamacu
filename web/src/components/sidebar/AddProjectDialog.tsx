import { useState, type FormEvent } from "react";
import { Loader2 } from "lucide-react";
import { useNavigate } from "react-router";
import { ApiError } from "@/api/client";
import { useCreateProject } from "@/api/mutations";
import { useSettings } from "@/api/settings";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";

export interface AddProjectDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

type Mode = "repo" | "folder";

/** Uppercase the first letter so server messages render in sentence case. */
function sentenceCase(message: string): string {
  return message.charAt(0).toUpperCase() + message.slice(1);
}

/**
 * Derive the project-name suggestion from a typed `owner/name` ref: the segment
 * after the last "/". Pure local string parse — no `gh`/network call (D-09).
 * Returns "" when there's no usable name segment.
 */
function deriveName(ownerName: string): string {
  const trimmed = ownerName.trim();
  const slash = trimmed.lastIndexOf("/");
  if (slash === -1) return "";
  return trimmed.slice(slash + 1);
}

export function AddProjectDialog({ open, onOpenChange }: AddProjectDialogProps) {
  const { data: settings } = useSettings();
  const integrationOn = settings?.github_integration?.value === "on";

  // Repo-first is the default selection when integration is on (D-01); folder
  // is the only mode when integration is off (D-02).
  const [mode, setMode] = useState<Mode>("repo");
  const [ownerName, setOwnerName] = useState("");
  const [repoPath, setRepoPath] = useState("");
  const [name, setName] = useState("");
  // Tracks whether the user has typed in the Name field, so the owner/name
  // prefill never clobbers an edit (mirrors ProjectSettingsDialog's repoEdited).
  const [nameEdited, setNameEdited] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const createProject = useCreateProject();
  const navigate = useNavigate();

  // The effective mode: when integration is off the dialog is folder-only, so
  // the repo-first branch never mounts regardless of `mode`.
  const activeMode: Mode = integrationOn ? mode : "folder";

  // Reset state on each open (and prefill the Name from a valid-looking
  // owner/name) by adjusting state during render keyed on the values that
  // changed — React's "adjust state on prop change" pattern, so no
  // setState-in-effect. `prevOpen`/`prevOwnerName` are the change trackers.
  const [prevOpen, setPrevOpen] = useState(open);
  const [prevOwnerName, setPrevOwnerName] = useState(ownerName);
  if (open !== prevOpen) {
    setPrevOpen(open);
    setPrevOwnerName(ownerName);
    if (open) {
      // Reset each time the dialog opens. Default to repo mode when integration
      // is on; folder is the only mode when it's off.
      setMode("repo");
      setOwnerName("");
      setRepoPath("");
      setName("");
      setNameEdited(false);
      setError(null);
    }
  } else if (ownerName !== prevOwnerName) {
    setPrevOwnerName(ownerName);
    // Prefill the Name from the owner/name's name segment while the user hasn't
    // edited it (D-04). Pure local parse, never clobbers a user edit.
    if (!nameEdited) {
      setName(deriveName(ownerName));
    }
  }

  function handleOpenChange(next: boolean) {
    if (!next) setError(null);
    onOpenChange(next);
  }

  function handleModeChange(next: string) {
    setMode(next as Mode);
    setError(null);
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    try {
      const project = await createProject.mutateAsync(
        activeMode === "repo"
          ? { repo: ownerName.trim(), name: name.trim() || undefined }
          : { repo_path: repoPath.trim(), name: name.trim() || undefined },
      );
      onOpenChange(false);
      setMode("repo");
      setOwnerName("");
      setRepoPath("");
      setName("");
      setNameEdited(false);
      navigate(`/projects/${project.id}`);
    } catch (err) {
      if (err instanceof ApiError) {
        // Mirror the server message verbatim, sentence-cased. A 409 ("already
        // added") gets a trailing period to read as a full sentence. Phase-14
        // atomicity guarantees no orphan row / no partial dir, so the copy
        // never implies a half-created project to clean up — it's just the
        // server's reason, and the user can correct and retry.
        let message = sentenceCase(err.message);
        if (err.status === 409 && !message.endsWith(".")) message += ".";
        setError(message);
      } else {
        setError("Couldn't add the project. Try again.");
      }
      // Field values + active mode are intentionally kept so the input can be
      // corrected and resubmitted.
    }
  }

  const submitDisabled =
    createProject.isPending ||
    (activeMode === "repo"
      ? ownerName.trim() === ""
      : repoPath.trim() === "");

  // Blocking clone-in-progress affordance (repo mode only): a muted spinner +
  // "Cloning <owner/name>…" while the synchronous create runs (D-06).
  const cloning = activeMode === "repo" && createProject.isPending;

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[480px]" aria-describedby={undefined}>
        <DialogHeader>
          <DialogTitle>Add project</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          {integrationOn && (
            <Tabs value={mode} onValueChange={handleModeChange}>
              <TabsList aria-label="Project source" className="w-full">
                <TabsTrigger
                  value="repo"
                  disabled={createProject.isPending}
                >GitHub repo</TabsTrigger>
                <TabsTrigger
                  value="folder"
                  disabled={createProject.isPending}
                >Local folder</TabsTrigger>
              </TabsList>
            </Tabs>
          )}

          {activeMode === "repo" ? (
            <div className="flex flex-col gap-1.5">
              <label htmlFor="add-project-repo" className="text-xs font-medium">
                Repository
              </label>
              <Input
                id="add-project-repo"
                className="font-mono"
                placeholder="owner/name"
                value={ownerName}
                onChange={(event) => setOwnerName(event.target.value)}
                disabled={createProject.isPending}
                autoFocus
              />
              {/* Exactly one of {error | help} shows. */}
              {error !== null ? (
                <p className="rounded-md border border-destructive/50 bg-destructive/10 px-3 py-2 text-xs text-destructive">
                  {error}
                </p>
              ) : (
                <p className="text-xs text-muted-foreground">
                  {`Enter a GitHub repo to clone — `}
                  <span className="font-mono">owner/name</span>
                  {`.`}
                </p>
              )}
            </div>
          ) : (
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
                disabled={createProject.isPending}
                autoFocus
              />
              {error !== null && (
                <p className="rounded-md border border-destructive/50 bg-destructive/10 px-3 py-2 text-xs text-destructive">
                  {error}
                </p>
              )}
            </div>
          )}

          <div className="flex flex-col gap-1.5">
            <label htmlFor="add-project-name" className="text-xs font-medium">
              Name
            </label>
            <Input
              id="add-project-name"
              placeholder={
                activeMode === "repo"
                  ? "Defaults to repo name"
                  : "Defaults to folder name"
              }
              value={name}
              onChange={(event) => {
                setNameEdited(true);
                setName(event.target.value);
              }}
              disabled={createProject.isPending}
            />
          </div>

          {cloning && (
            <p className="flex items-center gap-2 text-sm text-muted-foreground">
              <Loader2 className="size-4 animate-spin motion-reduce:animate-none" />
              <span>
                {`Cloning `}
                <span className="font-mono">{ownerName.trim()}</span>
                {`…`}
              </span>
            </p>
          )}

          <DialogFooter>
            <Button
              type="button"
              variant="ghost"
              onClick={() => handleOpenChange(false)}
              disabled={createProject.isPending}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={submitDisabled}>
              Add project
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
