import { useState } from "react";
import { useNavigate } from "react-router";
import { useAgents } from "@/api/queries";
import { useGlobal, useSaveGlobal } from "@/api/global";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";

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
      {/* Task 2: the Change-root dialog mounts here (D-43) — this task owns
          only the open state + the opening button. */}
    </section>
  );
}
