import { useState, type FormEvent } from "react";
import { Loader2 } from "lucide-react";
import { useNavigate } from "react-router";
import { ApiError, type ApiErrorReason } from "@/api/client";
import { useAgents } from "@/api/queries";
import { useGlobal, useSaveGlobal } from "@/api/global";
import { useSettings } from "@/api/settings";
import { Badge } from "@/components/ui/badge";
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs";

/** Uppercase the first letter so server messages render in sentence case
 *  (AddProjectDialog.tsx:27-29 — copied verbatim, D-45). */
function sentenceCase(message: string): string {
  return message.charAt(0).toUpperCase() + message.slice(1);
}

type Mode = "repo" | "folder";

/**
 * The Settings Scratchpad section (GCONF-05, D-43..D-46): a summary card
 * (root display + default-agent selector + the Open Scratchpad reachability
 * CTA) plus the Change-root dialog. Every row is server-truth-driven off the
 * ONE shared ["global"] query (16-01 useGlobal, D-17) — no local mirrors.
 */
export function ScratchpadSection() {
  const { data: global } = useGlobal();
  const { data: agents } = useAgents();
  const saveAgent = useSaveGlobal();
  const navigate = useNavigate();
  // Inline error for the instant-save selector only (Pitfall 10: on failure
  // the cache is untouched, so the controlled Select snaps back on its own).
  const [agentError, setAgentError] = useState(false);
  const [rootDialogOpen, setRootDialogOpen] = useState(false);

  const rootConfigured = !!global && global.root_path !== "";

  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{`Scratchpad`}</h2>
      {/* Composed summary card (D-44) — deliberately NOT a shadcn add; no
          card primitive exists in web/src/components/ui. */}
      <div className="flex flex-col gap-4 rounded-lg border bg-card p-4">
        {/* Row 1 — Root. The non-null github_repo pointer IS the managed
            marker (00017 contract); there is no boolean to read. */}
        <div className="flex flex-col gap-1.5">
          <Label className="text-xs font-medium">{`Root`}</Label>
          {global?.github_repo != null ? (
            <div className="flex min-w-0 items-center gap-2">
              <span className="font-mono text-sm truncate" title={global.github_repo}>
                {global.github_repo}
              </span>
              <Badge variant="secondary">{`managed`}</Badge>
            </div>
          ) : rootConfigured ? (
            <span className="font-mono text-sm truncate" title={global.root_path}>
              {global.root_path}
            </span>
          ) : (
            <p className="text-sm text-muted-foreground">{`No root configured yet.`}</p>
          )}
        </div>
        {/* Row 2 — Default agent (D-46). Instant save via PUT {agent_id};
            never 409-gated (D-24), so it lives OUTSIDE the root dialog. The
            Select value derives from the cached agent_id — never a state
            mirror — so a failed save snaps back automatically (Pitfall 10). */}
        <div className="flex flex-col gap-1.5">
          <Label className="text-xs font-medium">{`Default agent`}</Label>
          <Select
            value={global ? String(global.agent_id) : undefined}
            onValueChange={(value) => {
              setAgentError(false);
              saveAgent.mutate(
                { agent_id: Number(value) },
                { onError: () => setAgentError(true) },
              );
            }}
          >
            <SelectTrigger className="w-full">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {(agents ?? []).map((a) => (
                <SelectItem key={a.id} value={String(a.id)}>
                  {a.name}
                  {a.is_default ? " (default)" : ""}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {agentError ? (
            <p className="text-xs text-destructive">{`Couldn't save. Try again.`}</p>
          ) : (
            <p className="text-xs text-muted-foreground">{`Applies at the next Start.`}</p>
          )}
        </div>
        {/* Row 3 — Actions. Open Scratchpad is the section's single primary
            CTA (GCONF-05 idle reachability → /global). */}
        <div className="flex items-center gap-2">
          <Button onClick={() => navigate("/global")}>{`Open Scratchpad`}</Button>
          <Button
            variant="outline"
            aria-expanded={rootDialogOpen}
            onClick={() => setRootDialogOpen(true)}
          >
            {rootConfigured ? `Change root…` : `Configure root…`}
          </Button>
        </div>
      </div>
      <ChangeRootDialog
        open={rootDialogOpen}
        onOpenChange={setRootDialogOpen}
        configured={rootConfigured}
      />
    </section>
  );
}

/**
 * The Change-root dialog (D-43 — AddProjectDialog pattern verbatim, minus the
 * Name field/prefill machinery): segmented repo|folder capture gated on the
 * GitHub integration, blocking clone feedback, verbatim 409-reasons
 * surfacing, and the gated Clear-root action (D-44). EVERY failure keeps the
 * dialog open with field values and active mode preserved — only a 2xx closes
 * (degrade-don't-break; the mutation already wrote the GET-shaped response
 * into the ["global"] cache on success, D-23).
 */
function ChangeRootDialog({
  open,
  onOpenChange,
  configured,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  configured: boolean;
}) {
  const { data: settings } = useSettings();
  const integrationOn = settings?.github_integration?.value === "on";
  const save = useSaveGlobal();

  const [mode, setMode] = useState<Mode>("repo");
  const [ownerName, setOwnerName] = useState("");
  const [repoPath, setRepoPath] = useState("");
  const [error, setError] = useState<{
    message: string;
    reasons?: ApiErrorReason[];
  } | null>(null);

  // The effective mode: when integration is off the dialog is folder-only
  // (AddProjectDialog :67 degradation).
  const activeMode: Mode = integrationOn ? mode : "folder";

  // Reset state on each open by adjusting state during render keyed on the
  // prop change — React's "adjust state on prop change" pattern, so no
  // setState-in-effect (Pitfall 8; AddProjectDialog :73-95 idiom).
  const [prevOpen, setPrevOpen] = useState(open);
  if (open !== prevOpen) {
    setPrevOpen(open);
    if (open) {
      // Default to repo mode when integration is on; folder is the only
      // mode when it's off.
      setMode("repo");
      setOwnerName("");
      setRepoPath("");
      setError(null);
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

  // ApiError → the server body verbatim (sentenceCase lead only) plus the
  // structured 409 reasons when present; anything else → the canonical
  // retry copy. NO client re-wording either way (D-45).
  function saveError(err: unknown) {
    if (err instanceof ApiError) {
      setError({
        message: sentenceCase(err.message),
        reasons: err.reasons,
      });
    } else {
      setError({ message: "Couldn't save. Try again." });
    }
  }

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    try {
      await save.mutateAsync(
        activeMode === "repo"
          ? { repo: ownerName.trim() }
          : { root_path: repoPath.trim() },
      );
      onOpenChange(false);
    } catch (err) {
      saveError(err);
      // Field values + active mode are intentionally kept so the input can
      // be corrected and resubmitted.
    }
  }

  // D-44: root_path "" is THE one clear spelling, behind the same 409 gate.
  // NO nested confirmation dialog — the explicit button is the intent, the
  // gate is the guard.
  async function handleClearRoot() {
    setError(null);
    try {
      await save.mutateAsync({ root_path: "" });
      onOpenChange(false);
    } catch (err) {
      saveError(err);
    }
  }

  const submitDisabled =
    save.isPending ||
    (activeMode === "repo"
      ? ownerName.trim() === ""
      : repoPath.trim() === "");

  // Blocking clone-in-progress affordance (repo mode only): a muted spinner
  // + "Cloning <owner/name>…" while the synchronous repo PUT runs (D-06
  // idiom, AddProjectDialog :250-259).
  const cloning = activeMode === "repo" && save.isPending;

  // The error box (with the 409 reasons list) renders below the active
  // input whenever a failure is pending resolution — submit OR clear-root.
  const errorBox =
    error !== null ? (
      <div className="rounded-md border border-destructive/50 bg-destructive/10 px-3 py-2 text-xs text-destructive">
        <p>{error.message}</p>
        {error.reasons != null && error.reasons.length > 0 ? (
          <ul className="list-disc pl-4">
            {error.reasons.map((r) => (
              <li key={`${r.kind}-${r.target}`}>
                <span className="font-mono">{r.target}</span>
              </li>
            ))}
          </ul>
        ) : null}
      </div>
    ) : null;

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[480px]" aria-describedby={undefined}>
        <DialogHeader>
          <DialogTitle>Scratchpad root</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          {integrationOn && (
            <Tabs value={mode} onValueChange={handleModeChange}>
              <TabsList aria-label="Scratchpad root source" className="w-full">
                <TabsTrigger value="repo" disabled={save.isPending}>
                  GitHub repo
                </TabsTrigger>
                <TabsTrigger value="folder" disabled={save.isPending}>
                  Local folder
                </TabsTrigger>
              </TabsList>
            </Tabs>
          )}

          {activeMode === "repo" ? (
            <div className="flex flex-col gap-1.5">
              <label htmlFor="scratchpad-root-repo" className="text-xs font-medium">
                Repository
              </label>
              <Input
                id="scratchpad-root-repo"
                className="font-mono"
                placeholder="owner/name"
                value={ownerName}
                onChange={(event) => setOwnerName(event.target.value)}
                disabled={save.isPending}
                autoFocus
              />
              {/* Exactly one of {error | help} shows. */}
              {errorBox ?? (
                <p className="text-xs text-muted-foreground">
                  {`Enter a GitHub repo to clone — `}
                  <span className="font-mono">owner/name</span>
                  {`.`}
                </p>
              )}
            </div>
          ) : (
            <div className="flex flex-col gap-1.5">
              <label htmlFor="scratchpad-root-path" className="text-xs font-medium">
                Repository path
              </label>
              <Input
                id="scratchpad-root-path"
                className="font-mono"
                placeholder="/home/you/code/my-repo"
                value={repoPath}
                onChange={(event) => setRepoPath(event.target.value)}
                disabled={save.isPending}
                autoFocus
              />
              {errorBox}
            </div>
          )}

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
            {configured && (
              <Button
                type="button"
                variant="destructive"
                className="sm:mr-auto"
                onClick={handleClearRoot}
                disabled={save.isPending}
              >
                Clear root
              </Button>
            )}
            <Button
              type="button"
              variant="ghost"
              onClick={() => handleOpenChange(false)}
              disabled={save.isPending}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={submitDisabled}>
              Save root
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
