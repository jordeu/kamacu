import type { ReactNode } from "react";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useSettings } from "@/api/settings";
import { SettingsField } from "@/components/settings/SettingsField";

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

const AGENT_HELP = `Appended to every claude spawn, starting with the next Start agent. Remove --dangerously-skip-permissions to restore interactive permission prompts.`;
const WORKTREE_HELP = `New task worktrees are created under this directory. Existing worktrees stay where they are.`;
const SHELL_HELP = `Used when opening a new bash tab.`;
const BRANCH_HELP = `Tokens: {slug}, {id}, {title}. Applied when a task is created — e.g. task/fix-login-42.`;

export default function SettingsPage() {
  const { data: settings, isLoading, isError, refetch } = useSettings();

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
      <div className="max-w-[640px]">
        <h1 className="text-base font-medium">{`Settings`}</h1>
        {/* SET-03 stated once for the whole page, never per field. */}
        <p className="mt-1 text-xs text-muted-foreground">
          {`Changes apply to the next session or worktree — running sessions are unaffected.`}
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
          <div className="mt-6 flex flex-col gap-6">
            <section className="flex flex-col gap-3">
              <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">{`Agent`}</h2>
              {/* No warning chrome — the configured default renders like any
                  other field (D-51 reversal is specified behavior). */}
              <SettingsField
                settingKey="agent_extra_params"
                label={`Extra claude parameters`}
                entry={settings.agent_extra_params}
                mono
                help={withMono(AGENT_HELP, ["--dangerously-skip-permissions"])}
              />
            </section>
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
          </div>
        )}
      </div>
    </div>
  );
}
