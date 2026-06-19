import { useState, type FormEvent } from "react";
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
import { ProjectAvatar } from "@/components/ui/ProjectAvatar";
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
 * Mirror the server's validateIconLetters rule (internal/api/icons.go, D-10) as
 * client-side UX: trim, keep up to 2 alphanumeric (Unicode letter or digit)
 * runes, then uppercase — symbols/emoji/whitespace are stripped. The server
 * stays the enforcer; this only shapes what the user can type so the avatar
 * preview matches what will be stored.
 */
function normalizeIconLetters(raw: string): string {
  const alnum = raw.match(/[\p{L}\p{N}]/gu) ?? [];
  return alnum.slice(0, 2).join("").toUpperCase();
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
  const [letters, setLetters] = useState(project.icon_letters);
  const [color, setColor] = useState(project.icon_color);
  // Tracks whether the user has typed in the repo field, so a late-arriving
  // origin suggestion never clobbers an edit.
  const [repoEdited, setRepoEdited] = useState(false);
  const [error, setError] = useState<string | null>(null);

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
      setLetters(project.icon_letters);
      setColor(project.icon_color);
      setRepoEdited(false);
      setError(null);
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
    // Send github_repo ONLY when the user actually changed it (and the field is
    // visible). An unchanged repo OR integration off → omit it → the backend
    // leaves the link untouched, so a description-only edit never re-validates
    // and can't be hard-blocked by a transient gh hiccup.
    const repoChanged =
      integrationOn && repo.trim() !== (project.github_repo ?? "");
    // Send each icon field ONLY when it changed (same omitted-=-untouched shape
    // as github_repo; mutations.ts drops undefined keys). The server stays the
    // enforcer — an empty `letters` here surfaces a 400 inline below (D-12).
    const lettersChanged = letters !== project.icon_letters;
    try {
      await updateSettings.mutateAsync({
        id: project.id,
        description,
        github_repo: repoChanged ? repo.trim() : undefined,
        icon_letters: lettersChanged ? letters : undefined,
      });
      // A 2xx save always closes the dialog.
      onOpenChange(false);
    } catch (err) {
      if (err instanceof ApiError && err.status < 500) {
        // Mandatory-validation reject (400): a syntactically invalid OR
        // gh-unverifiable ref. Highlighted destructive alert under the repo
        // field, dialog STAYS OPEN, the description draft is preserved.
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
          {/* Live preview (D-09): reflects the in-progress draft, not the saved
              project, so it updates as the user edits letters/color. */}
          <div className="flex justify-center">
            <ProjectAvatar size="rail" letters={letters} color={color} />
          </div>

          <div className="flex flex-col gap-2">
            <Label
              htmlFor={`project-letters-${project.id}`}
              className="text-xs font-medium"
            >
              Initials
            </Label>
            <Input
              id={`project-letters-${project.id}`}
              maxLength={2}
              value={letters}
              autoCapitalize="characters"
              onChange={(event) => setLetters(normalizeIconLetters(event.target.value))}
            />
            <p className="text-xs text-muted-foreground">
              Two letters shown on the project avatar.
            </p>
          </div>

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
            {!error && (
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
                }}
              />
              {/* Exactly one of {error | help} shows. */}
              {error !== null ? (
                <p className="rounded-md border border-destructive/50 bg-destructive/10 px-3 py-2 text-xs text-destructive">
                  {error}
                </p>
              ) : (
                <p className="text-xs text-muted-foreground">{repoHelp}</p>
              )}
            </div>
          )}

          {/* Form-level error when the repo field is hidden (integration off). */}
          {!integrationOn && error !== null && (
            <p className="rounded-md border border-destructive/50 bg-destructive/10 px-3 py-2 text-xs text-destructive">
              {error}
            </p>
          )}

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
              disabled={updateSettings.isPending || letters.trim() === ""}
            >
              Save changes
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
