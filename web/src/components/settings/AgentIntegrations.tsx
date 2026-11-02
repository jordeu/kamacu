import { useState } from "react";
import { useIntegrations, useUpdateIntegration, type IntegrationEngine, type IntegrationStatus } from "@/api/integrations";
import { Button } from "@/components/ui/button";

const names: Record<IntegrationEngine, string> = { claude: "Claude Code", codex: "Codex", opencode: "OpenCode" };

function description(item: IntegrationStatus) {
  if (item.message) return item.message;
  if (!item.available) return "Install this agent on the Kamacu host to enable integration.";
  if (item.mcp === "modified") return "Your edits are preserved. Check the MCP entry in your agent configuration before repairing.";
  if (item.mcp === "conflict") return "A different Kamacu MCP registration already exists. Check your agent configuration.";
  if (!item.enabled) return "Enable MCP tools for previews and other Kamacu features.";
  if (item.mcp === "registered") return "Enabled · MCP registered.";
  return "Setup needs attention. Repair restores missing MCP registrations and updates managed entries.";
}

export function AgentIntegrations() {
  const query = useIntegrations();
  const update = useUpdateIntegration();
  const [notice, setNotice] = useState<{ engine: IntegrationEngine; text: string; error: boolean } | null>(null);

  async function change(engine: IntegrationEngine, action: "enable" | "repair" | "remove") {
    setNotice(null);
    try {
      const result = await update.mutateAsync({ engine, action });
      setNotice({ engine, error: false, text: result.message || (action === "remove" ? "Managed MCP registration removed. Restart existing sessions." : "Integration ready. Start a new task agent session to load the MCP tools.") });
    } catch (error) {
      setNotice({ engine, error: true, text: error instanceof Error ? error.message : "Could not update integration. Try again." });
    }
  }

  return (
    <div className="mt-3 flex flex-col gap-3 border-t pt-4">
      <h3 className="text-sm font-medium">Kamacu integration</h3>
      <p className="text-xs text-muted-foreground">
        Registers optional MCP tools for previews and other Kamacu features. Terminal monitoring handles inbox notifications automatically.
        Managed registrations update with Kamacu; your edits are preserved. No startup prompts are added.
      </p>
      {query.isPending && <p role="status" className="text-xs text-muted-foreground">Checking agent integrations…</p>}
      {query.isError && <div role="alert" className="text-xs text-destructive">
        Could not check integrations. {query.error.message}
        <Button variant="link" size="sm" onClick={() => query.refetch()}>Retry</Button>
      </div>}
      {query.data?.map(item => (
        <div key={item.engine} className="flex flex-col gap-2 py-2" aria-label={`${names[item.engine]} integration`}>
          <div className="flex flex-wrap items-center justify-between gap-2">
            <span className="text-sm font-medium">{names[item.engine]}</span>
            <div className="flex items-center gap-1">
              <Button size="sm" variant="outline" disabled={!item.available || update.isPending}
                aria-label={`${item.enabled ? "Repair" : "Enable"} ${names[item.engine]} integration`}
                onClick={() => change(item.engine, item.enabled ? "repair" : "enable")}>
                {update.isPending && update.variables?.engine === item.engine ? "Updating…" : item.enabled ? "Repair" : "Enable integration"}
              </Button>
              {(item.managed || item.enabled || item.mcp === "modified") && <Button size="sm" variant="ghost" disabled={update.isPending}
                aria-label={`Remove ${names[item.engine]} integration`} onClick={() => change(item.engine, "remove")}>Remove</Button>}
            </div>
          </div>
          <p className="text-xs text-muted-foreground break-words">{description(item)}</p>
          {notice?.engine === item.engine && <p role={notice.error ? "alert" : "status"} className={`text-xs break-words ${notice.error ? "text-destructive" : "text-muted-foreground"}`}>{notice.text}</p>}
        </div>
      ))}
    </div>
  );
}
