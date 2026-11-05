import { useState, type ReactNode } from "react";
import { useSearchParams } from "react-router";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { useSaveSetting, useSettings, type Settings } from "@/api/settings";
import { useGithubStatus } from "@/api/queries";
import { SettingsField } from "@/components/settings/SettingsField";
import { WorktreeCleanupSection } from "@/components/settings/WorktreeCleanupSection";
import { AgentsSection } from "@/components/settings/AgentsSection";
import { NotificationsSection } from "@/components/settings/NotificationsSection";
import { MonitoringSection } from "@/components/settings/MonitoringSection";
import { ScratchpadSection } from "@/components/settings/ScratchpadSection";

/**
 * Wraps the given substrings of a contract literal in the mono stack at
 * render time, so every UI-SPEC string stays a single grep-able template
 * literal in source (Phase 3 convention) while flags/tokens/examples still
 * render in font-mono.
 */
function withMono(text: string, tokens: string[]): ReactNode {
  let parts: ReactNode[] = [text];
  for (const token of tokens) {
    parts = parts.flatMap((part, partIndex) => {
      if (typeof part !== "string" || !part.includes(token)) return [part];
      const out: ReactNode[] = [];
      part.split(token).forEach((piece, i) => {
        if (i > 0) {
          out.push(
            <span key={`${token}-${partIndex}-${i}`} className="font-mono">
              {token}
            </span>,
          );
        }
        if (piece !== "") out.push(piece);
      });
      return out;
    });
  }
  return parts;
}

const WORKTREE_HELP = `New task worktrees are created under this directory. Existing worktrees stay where they are.`;
const SHELL_HELP = `Used when opening a new bash tab.`;
const BRANCH_HELP = `Tokens: {slug}, {id}, {title}. Applied when a task is created — e.g. task/fix-login-42.`;
const DONE_TTL_HELP = `Sessions of tasks left in Done are killed after this idle time. Use a duration like 24h or 90m; clear the field or enter never to disable. Worktrees are never removed.`;
const GITHUB_INTEGRATION_HELP = `Show GitHub features across Kamacu. Turn off to hide all GitHub UI.`;
const GITHUB_GH_MISSING_HELP = `Install the GitHub CLI (gh) before enabling GitHub integration.`;
const PR_REVIEW_SEED_HELP = `Prefilled (not sent) into the agent when you Start a review. Use <n> for the PR number and <title> for the PR title. Leave blank to start with no prompt.`;

/**
 * The GitHub integration on/off Switch (GHSET-01/GHSET-03). A Switch has no
 * draft — it commits immediately via the shared settings KV (PUT
 * /api/settings/github_integration). The cache is replaced on success
 * (useSaveSetting), flipping `enabled`; on failure the cache is untouched so
 * `checked` falls back to the last saved value (never sticks mid-flip) and the
 * canonical error renders under the row. This section stays available in the
 * GitHub tab even when integration is off, so users can enable it again.
 *
 * The toggle is gh-aware: it reads useGithubStatus() (GET /api/github/status).
 * When gh is unavailable on the host, the EFFECTIVE checked state is forced off
 * (never shown on, even if a stale stored row says "on"), and an enable attempt
 * is blocked — instead of flipping on, it surfaces install-gh guidance. A
 * still-loading or errored status query is treated as "not available" so the
 * toggle never falsely shows on before availability is known.
 */
function GithubSection({
  enabled,
  settings,
}: {
  enabled: boolean;
  settings: Settings;
}) {
  const save = useSaveSetting("github_integration");
  const [error, setError] = useState<string | null>(null);
  const { data: ghStatus } = useGithubStatus();
  const ghAvailable = ghStatus?.gh_available === true;
  const effectiveEnabled = enabled && ghAvailable;

  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{`GitHub`}</h2>
      <div className="flex items-center gap-2">
        <Switch
          id="github-integration"
          aria-label={`GitHub integration`}
          checked={effectiveEnabled}
          onCheckedChange={(checked) => {
            if (checked && !ghAvailable) {
              setError(GITHUB_GH_MISSING_HELP);
              return;
            }
            save.mutate(checked ? "on" : "off", {
              onSuccess: () => setError(null),
              onError: () => setError(`Couldn't save. Try again.`),
            });
          }}
        />
        <Label htmlFor="github-integration" className="text-sm font-medium">
          {`GitHub integration`}
        </Label>
      </div>
      {error !== null ? (
        <p className="text-xs text-destructive">{error}</p>
      ) : (
        <p className="text-xs text-muted-foreground">
          {ghAvailable ? GITHUB_INTEGRATION_HELP : GITHUB_GH_MISSING_HELP}
        </p>
      )}
      {/* PR review prompt (12-07 fix #7): the configurable seed bound to
          pr_review_seed. Lives in the section the toggle controls, so it only
          shows while integration is enabled. Default-prefilled from the stored
          value or the code default; edit + blur persists via the settings PUT
          the review view (TaskPage) then reads. */}
      {effectiveEnabled && settings.pr_review_seed && (
        <SettingsField
          settingKey="pr_review_seed"
          label={`PR review prompt`}
          entry={settings.pr_review_seed}
          control="textarea"
          help={withMono(PR_REVIEW_SEED_HELP, ["<n>", "<title>"])}
        />
      )}
    </section>
  );
}

const SETTINGS_GROUPS = [
  { id: "workspace", label: "Workspace", description: "Choose where tasks run and how shells and branches are created." },
  { id: "agents", label: "Agents", description: "Manage your coding agents and the scratchpad they use." },
  { id: "monitoring", label: "Monitoring", description: "Configure AI profiles and workspace monitoring." },
  { id: "github", label: "GitHub", description: "Manage GitHub integration, review prompts, and notification sources." },
  { id: "cleanup", label: "Cleanup", description: "Control idle sessions and clean up task worktrees." },
] as const;

export default function SettingsPage() {
  const { data: settings, isLoading, isError, refetch } = useSettings();
  const [params, setParams] = useSearchParams();
  const activeGroup = SETTINGS_GROUPS.find(group => group.id === params.get("section"))?.id ?? "workspace";

  function selectGroup(value: string) {
    setParams(previous => {
      const next = new URLSearchParams(previous);
      next.set("section", value);
      return next;
    });
  }

  if (isError) {
    // Mirrors the board load-failure pattern.
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3">
        <p className="text-muted-foreground">{`Couldn't load settings.`}</p>
        <Button variant="outline" onClick={() => refetch()}>
          {`Retry loading`}
        </Button>
      </div>
    );
  }

  return (
    <div className="h-full overflow-y-auto p-4">
      <div className="mx-auto w-full max-w-[640px]">
        <h1 className="text-base font-medium">{`Settings`}</h1>
        {/* SET-03 stated once for the whole page, never per field. */}
        <p className="mt-1 text-xs text-muted-foreground">
          {`Inline fields save when you leave them; switches and menus save immediately. Use the save controls in dialogs. Monitoring changes apply immediately; session and worktree defaults apply to new sessions and worktrees.`}
        </p>
        {isLoading || !settings ? (
          <div className="mt-6 flex flex-col gap-6">
            {[0, 1, 2, 3].map((i) => (
              <div key={i} className="flex flex-col gap-2">
                <Skeleton className="h-3 w-24" />
                <Skeleton className="h-8 w-full" />
              </div>
            ))}
          </div>
        ) : (
          <Tabs value={activeGroup} onValueChange={selectGroup} activationMode="manual" className="mt-6 gap-6">
            <TabsList aria-label="Settings categories" className="sticky top-0 z-10 h-auto! w-full flex-wrap justify-start gap-1 border bg-background shadow-sm">
              {SETTINGS_GROUPS.map(group => (
                <TabsTrigger key={group.id} value={group.id} className="min-h-9 px-3">
                  {group.label}
                </TabsTrigger>
              ))}
            </TabsList>
            {/* Keep panels mounted so drafts and save feedback survive tab changes. */}
            <TabsContent value="workspace" forceMount hidden={activeGroup !== "workspace"} className="space-y-6">
              <p className="text-sm text-muted-foreground">{SETTINGS_GROUPS.find(group => group.id === "workspace")?.description}</p>
              <section className="flex flex-col gap-3">
                <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{`Worktrees`}</h2>
                <SettingsField
                  settingKey="worktree_base"
                  label={`Worktree base directory`}
                  entry={settings.worktree_base}
                  mono
                  help={WORKTREE_HELP}
                />
              </section>
              <section className="flex flex-col gap-3">
                <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{`Bash tabs`}</h2>
                {/* Dropdown options come from entry.options in the API payload —
                    never a frontend constant (the SHELL-02 seam). */}
                <SettingsField
                  settingKey="shell"
                  label={`Shell`}
                  entry={settings.shell}
                  control="select"
                  help={SHELL_HELP}
                />
              </section>
              <section className="flex flex-col gap-3">
                <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{`Branches`}</h2>
                <SettingsField
                  settingKey="branch_template"
                  label={`Branch name template`}
                  entry={settings.branch_template}
                  mono
                  help={withMono(BRANCH_HELP, [
                    `{slug}`,
                    `{id}`,
                    `{title}`,
                    `task/fix-login-42`,
                  ])}
                />
              </section>
            </TabsContent>
            <TabsContent value="agents" forceMount hidden={activeGroup !== "agents"} className="space-y-6">
              <p className="text-sm text-muted-foreground">{SETTINGS_GROUPS.find(group => group.id === "agents")?.description}</p>
              <AgentsSection />
              <ScratchpadSection />
            </TabsContent>
            <TabsContent value="monitoring" forceMount hidden={activeGroup !== "monitoring"} className="space-y-6">
              <p className="text-sm text-muted-foreground">{SETTINGS_GROUPS.find(group => group.id === "monitoring")?.description}</p>
              <MonitoringSection />
            </TabsContent>
            <TabsContent value="github" forceMount hidden={activeGroup !== "github"} className="space-y-6">
              <p className="text-sm text-muted-foreground">{SETTINGS_GROUPS.find(group => group.id === "github")?.description}</p>
              <GithubSection
                enabled={settings.github_integration?.value === "on"}
                settings={settings}
              />
              <NotificationsSection settings={settings} />
            </TabsContent>
            <TabsContent value="cleanup" forceMount hidden={activeGroup !== "cleanup"} className="space-y-6">
              <p className="text-sm text-muted-foreground">{SETTINGS_GROUPS.find(group => group.id === "cleanup")?.description}</p>
              <section className="flex flex-col gap-3">
                <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{`Cleanup`}</h2>
                {/* REAP-01/D-91: read-at-use TTL; "never"/empty/0 disable reaping
                    (the disable copy here mirrors ParseDoneSessionTTL semantics). */}
                <SettingsField
                  settingKey="done_session_ttl"
                  label={`Done session TTL`}
                  entry={settings.done_session_ttl}
                  mono
                  help={DONE_TTL_HELP}
                />
              </section>
              <WorktreeCleanupSection />
            </TabsContent>
          </Tabs>
        )}
      </div>
    </div>
  );
}
