import { useState } from "react";
import { useIntegrations, useUpdateIntegration, type IntegrationEngine, type IntegrationStatus } from "@/api/integrations";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";

const names: Record<IntegrationEngine, string> = { claude: "Claude Code", codex: "Codex", opencode: "OpenCode", pi: "Pi" };
type Action = "enable" | "repair" | "remove";

function status(item: IntegrationStatus) {
  if (!item.available) return { label: "Not installed", description: "Install this agent on the Kamacu host to enable integration." };
  if (item.mcp === "modified" || item.mcp === "conflict") {
    return { label: "Check configuration", description: "Review the Kamacu tools registration in your agent configuration." };
  }
  if (item.mcp === "error") return { label: "Could not check setup", description: "Check your agent configuration and try again." };
  if (!item.enabled) return { label: "Off", description: "Enable project instructions, skills and Kamacu tools." };
  if (item.mcp === "registered" && !item.message) return { label: "Enabled", description: "Project context and Kamacu tools are ready." };
  return { label: "Needs repair", description: "Repair restores missing registrations and updates managed entries." };
}

export function AgentIntegrations() {
  const query = useIntegrations();
  const update = useUpdateIntegration();
  const [notice, setNotice] = useState<{ engine: IntegrationEngine; text: string; error: boolean } | null>(null);

  async function change(engine: IntegrationEngine, action: Action) {
    setNotice(null);
    try {
      const result = await update.mutateAsync({ engine, action });
      setNotice({ engine, error: false, text: result.message || (action === "remove" ? "Integration removed. Restart existing sessions." : "Integration ready. Start a new task agent session to load project context and MCP tools.") });
    } catch (error) {
      setNotice({ engine, error: true, text: error instanceof Error ? error.message : "Could not update integration. Try again." });
    }
  }

  function actionButton(engine: IntegrationEngine, action: Action, label: string) {
    const pending = update.isPending && update.variables?.engine === engine && update.variables?.action === action;
    return (
      <Button size="sm" variant={action === "remove" ? "ghost" : "outline"} disabled={update.isPending}
        aria-label={`${label} ${names[engine]} integration`}
        onClick={() => change(engine, action)}>
        {pending ? "Updating…" : label}
      </Button>
    );
  }

  if (query.isPending) return <p role="status" className="text-sm text-muted-foreground">Checking agent integrations…</p>;
  if (query.isError) return (
    <div role="alert" className="space-y-2 text-sm text-destructive">
      <p>Could not check integrations. {query.error.message}</p>
      <Button variant="outline" size="sm" onClick={() => query.refetch()}>Retry</Button>
    </div>
  );

  return (
    <ul className="divide-y rounded-lg border">
      {query.data.map(item => {
        const state = status(item);
        const canRemove = item.managed || item.enabled || item.mcp === "modified";
        const checkConfig = ["modified", "conflict", "error"].includes(item.mcp);
        const needsRepair = item.available && item.enabled && !checkConfig && state.label === "Needs repair";
        const canEnable = item.available && !item.enabled && !checkConfig;
        const fallbackAction = item.available && !needsRepair && !canEnable;
        return (
          <li key={item.engine} className="space-y-2 p-4" aria-label={`${names[item.engine]} integration`}>
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="flex flex-wrap items-center gap-2">
                <h3 className="text-sm font-medium">{names[item.engine]}</h3>
                <Badge variant="secondary">{state.label}</Badge>
              </div>
              {canEnable && actionButton(item.engine, "enable", "Enable")}
              {needsRepair && actionButton(item.engine, "repair", "Repair")}
            </div>
            <p className="break-words text-sm text-muted-foreground">{item.message || state.description}</p>
            {(fallbackAction || canRemove) && (
              <details className="text-sm">
                <summary className="w-fit cursor-pointer py-1 text-muted-foreground">Manage integration</summary>
                <div className="mt-1 flex flex-wrap gap-2">
                  {fallbackAction && actionButton(item.engine, item.enabled ? "repair" : "enable", item.enabled ? "Repair" : "Enable")}
                  {canRemove && actionButton(item.engine, "remove", "Remove")}
                </div>
              </details>
            )}
            {notice?.engine === item.engine && (
              <p role={notice.error ? "alert" : "status"} className={`break-words text-sm ${notice.error ? "text-destructive" : "text-muted-foreground"}`}>
                {notice.text}
              </p>
            )}
          </li>
        );
      })}
    </ul>
  );
}
