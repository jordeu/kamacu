import { useState, type FormEvent, type ReactNode } from "react";
import { ApiError } from "@/api/client";
import { useUpdateProjectSettings } from "@/api/mutations";
import { useProjectGithubOrigin } from "@/api/queries";
import { useSettings } from "@/api/settings";
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
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";

export interface ProjectSettingsDialogProps {
  project: Project;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/** Description soft cap (UI-SPEC, D-07); the server also caps. */
const DESCRIPTION_CAP = 280;
/** Show the live counter only once the user gets close to the cap. */
const COUNTER_THRESHOLD = 240;

const REPO_HELP_WITH_ORIGIN = `Detected from this repo's origin remote. Accepts owner/name or a GitHub URL.`;
const REPO_HELP_NO_ORIGIN = `Accepts owner/name or a GitHub URL.`;

/** Uppercase the first letter so server messages render in sentence case. */
function sentenceCase(message: string): string {
  return message.charAt(0).toUpperCase() + message.slice(1);
}

/**
 * Soft advisory copy (D-11). These are non-blocking: the PATCH succeeded and
 * the value is stored. Plan 02's server returns a plain 200 for soft-saves
 * (no verify flag), so the success path closes the dialog and these are not
 * triggered by the current server — but the muted render path below IS wired
 * (the `warning` state renders these), so the advisories are present and
 * testable for a future server verify-flag.
 */

/** `Saved, but couldn't verify {owner/name} on GitHub — check access or the name.` */
function softVerifyWarning(repo: string): ReactNode {
  return (
    <>
      {`Saved, but couldn't verify `}
      <span className="font-mono">{repo}</span>
      {` on GitHub — check access or the name.`}
    </>
  );
}

/** `Saved without verifying — the gh CLI isn't available.` */
function ghDegradedWarning(): ReactNode {
  return (
    <>
      {`Saved without verifying — the `}
      <span className="font-mono">{`gh`}</span>
      {` CLI isn't available.`}
    </>
  );
}

export function ProjectSettingsDialog({
  project,
  open,
  onOpenChange,
}: ProjectSettingsDialogProps) {
  const { data: settings } = useSettings();
  const integrationOn = settings?.github_integration?.value === "on";

  const [description, setDescription] = useState(project.description);
  const [repo, setRepo] = useState(project.github_repo ?? "");
  // Tracks whether the user has typed in the repo field, so a late-arriving
  // origin suggestion never clobbers an edit.
  const [repoEdited, setRepoEdited] = useState(false);
  const [error, setError] = useState<string | null>(null);
  // Non-blocking advisory (muted) — wired for a future server verify-flag.
  const [warning, setWarning] = useState<ReactNode | null>(null);

  const updateSettings = useUpdateProjectSettings();

  // Origin prefill (D-08): fetched ONLY while the dialog is open AND the
  // integration is on — git is never shelled on the project list.
  const origin = useProjectGithubOrigin(project.id, open && integrationOn);
  const originSuggestion = origin.data?.suggestion ?? "";

  // Reset + prefill state on each open and prefill the late-arriving origin
  // suggestion — done by adjusting state during render keyed on the values
  // that changed (React's "adjust state on prop change" pattern), so no
  // setState-in-effect. `prevOpen`/`prevSuggestion` are the change trackers.
  const [prevOpen, setPrevOpen] = useState(open);
  const [prevSuggestion, setPrevSuggestion] = useState(originSuggestion);
  if (open !== prevOpen) {
    setPrevOpen(open);
    setPrevSuggestion(originSuggestion);
    if (open) {
      // Reset + prefill each time the dialog opens (RenameProjectDialog precedent).
      setDescription(project.description);
      setRepo(project.github_repo ?? "");
      setRepoEdited(false);
      setError(null);
      setWarning(null);
    }
  } else if (originSuggestion !== prevSuggestion) {
    setPrevSuggestion(originSuggestion);
    // When the origin suggestion resolves and the project is unlinked and the
    // user hasn't edited the field, prefill the suggestion.
    if (
      open &&
      integrationOn &&
      !repoEdited &&
      !project.github_repo &&
      originSuggestion !== ""
    ) {
      setRepo(originSuggestion);
    }
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    setWarning(null);
    const typed = integrationOn ? repo.trim() : (project.github_repo ?? "");
    try {
      const updated = await updateSettings.mutateAsync({
        id: project.id,
        description,
        // When integration is off the repo field is hidden — don't clobber the
        // saved link; send the existing value so the hidden field is preserved.
        github_repo: typed,
      });
      // Soft advisories never block closing (UI-SPEC). Plan 02's server returns
      // a plain 200 with no verify flag, so `verifyState` is always undefined
      // today and the dialog closes. This wires the muted advisory render path
      // for a future server verify flag without changing today's behavior.
      const verifyState = (updated as { verify_state?: string }).verify_state;
      if (verifyState === "no_gh") {
        setWarning(ghDegradedWarning());
      } else if (verifyState === "unverifiable" && updated.github_repo) {
        setWarning(softVerifyWarning(updated.github_repo));
      } else {
        onOpenChange(false);
      }
    } catch (err) {
      if (err instanceof ApiError && err.status < 500) {
        // Hard validation case (Plan 02 only 400s on a syntactically invalid
        // ref): destructive red under the repo field, dialog STAYS OPEN, the
        // description draft is preserved.
        setError(sentenceCase(err.message));
      } else {
        // Network / 5xx: form-level destructive copy, dialog stays open.
        setError(`Couldn't save. Try again.`);
      }
    }
  }

  const showCounter = description.length > COUNTER_THRESHOLD;
  const repoHelp = originSuggestion !== "" ? REPO_HELP_WITH_ORIGIN : REPO_HELP_NO_ORIGIN;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent aria-describedby={undefined}>
        <DialogHeader>
          <DialogTitle>Project settings</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div className="flex flex-col gap-2">
            <div className="flex items-center justify-between">
              <Label
                htmlFor={`project-description-${project.id}`}
                className="text-xs font-medium"
              >
                Description
              </Label>
              {showCounter && (
                <span className="text-xs text-muted-foreground">
                  {`${description.length}/${DESCRIPTION_CAP}`}
                </span>
              )}
            </div>
            <Textarea
              id={`project-description-${project.id}`}
              rows={3}
              value={description}
              placeholder="Optional short description"
              onChange={(event) => {
                const next = event.target.value;
                // Hard-stop input at the cap.
                if (next.length <= DESCRIPTION_CAP) setDescription(next);
              }}
            />
            {!error && !warning && (
              <p className="text-xs text-muted-foreground">{`Shown here in project settings.`}</p>
            )}
          </div>

          {integrationOn && (
            <div className="flex flex-col gap-2">
              <Label
                htmlFor={`project-repo-${project.id}`}
                className="text-xs font-medium"
              >
                GitHub repository
              </Label>
              <Input
                id={`project-repo-${project.id}`}
                className="font-mono"
                placeholder="owner/name"
                value={repo}
                onChange={(event) => {
                  setRepoEdited(true);
                  setRepo(event.target.value);
                  setError(null);
                  setWarning(null);
                }}
              />
              {/* Exactly one of {error | warning | help} shows. */}
              {error !== null ? (
                <p className="text-xs text-destructive">{error}</p>
              ) : warning !== null ? (
                <p className="text-xs text-muted-foreground">{warning}</p>
              ) : (
                <p className="text-xs text-muted-foreground">{repoHelp}</p>
              )}
            </div>
          )}

          {/* Form-level error when the repo field is hidden (integration off). */}
          {!integrationOn && error !== null && (
            <p className="text-xs text-destructive">{error}</p>
          )}

          <DialogFooter>
            <Button
              type="button"
              variant="ghost"
              onClick={() => onOpenChange(false)}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={updateSettings.isPending}>
              Save changes
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
