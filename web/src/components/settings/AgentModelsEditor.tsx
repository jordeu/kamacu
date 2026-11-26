import { useState } from "react";
import { post } from "@/api/client";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import type { Agent } from "@/api/types";

interface Props {
  engine: Agent["engine"];
  command: string;
  models: string[];
  onModelsChange: (models: string[]) => void;
  modelArgument: string;
  onModelArgumentChange: (argument: string) => void;
  discoveryCommand: string;
  onDiscoveryCommandChange: (command: string) => void;
}

export function AgentModelsEditor({ engine, command, models, onModelsChange, modelArgument, onModelArgumentChange, discoveryCommand, onDiscoveryCommandChange }: Props) {
  const source = JSON.stringify([engine, command, discoveryCommand]);
  const [discoverySource, setDiscoverySource] = useState<string | null>(null);
  const [draft, setDraft] = useState("");
  const [filter, setFilter] = useState("");
  const [suggestions, setSuggestions] = useState<string[]>([]);
  const [discovering, setDiscovering] = useState(false);
  const [discovered, setDiscovered] = useState(false);
  const [error, setError] = useState<string | null>(null);

  function add(model: string) {
    const id = model.trim();
    if (id && !models.includes(id)) onModelsChange([...models, id]);
  }

  async function discover() {
    setDiscoverySource(source);
    setDiscovering(true);
    setFilter("");
    setDiscovered(false);
    setSuggestions([]);
    setError(null);
    try {
      const result = await post<{ models: string[] }>("/api/agents/discover-models", {
        engine, command, model_discovery_command: discoveryCommand,
      });
      setSuggestions(result.models);
      setDiscovered(true);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not discover models. You can add them manually.");
    } finally {
      setDiscovering(false);
    }
  }

  const available = suggestions.filter((model) => !models.includes(model));
  const filtered = available.filter((model) => model.toLowerCase().includes(filter.toLowerCase()));

  return (
    <fieldset className="flex min-w-0 flex-col gap-3 border-t pt-3">
      <legend className="text-xs font-medium">Models</legend>
      <p className="text-xs text-muted-foreground">Choose which models appear when creating tasks. Discovery only suggests models; add the ones you want, then save.</p>
      {engine !== "pi" && <div className="flex flex-col gap-1.5">
        <label htmlFor="agent-model-argument" className="text-xs font-medium">Model argument</label>
        <Input id="agent-model-argument" value={modelArgument} onChange={(event) => onModelArgumentChange(event.target.value)} className="font-mono text-xs" placeholder="--model {{model}}" />
        <p className="text-xs text-muted-foreground">{"Use {{model}} for the selected model, for example --model {{model}} or --model={{model}}. Leave empty if this agent cannot select a model."}</p>
      </div>}
      <div className="flex gap-2">
        <Input aria-label="Model ID" value={draft} onChange={(event) => setDraft(event.target.value)} placeholder={engine === "pi" ? "provider/model-id" : "Model ID or alias"} className="font-mono text-xs" maxLength={256}
          onKeyDown={(event) => { if (event.key === "Enter" && !event.nativeEvent.isComposing) { event.preventDefault(); add(draft); setDraft(""); } }} />
        <Button type="button" variant="outline" disabled={!draft.trim() || models.includes(draft.trim())} onClick={() => { add(draft); setDraft(""); }}>Add model</Button>
      </div>
      {models.length > 0 ? <ul aria-label="Configured models" className="max-h-40 overflow-y-auto space-y-1">
        {models.map((model) => <li key={model} className="flex items-center justify-between gap-2 rounded border px-2 py-1">
          <span className="break-all font-mono text-xs">{model}</span>
          <Button type="button" size="sm" variant="ghost" aria-label={`Remove ${model}`} onClick={() => onModelsChange(models.filter((id) => id !== model))}>Remove</Button>
        </li>)}
      </ul> : <p className="text-xs text-muted-foreground">No models added. Tasks will use the agent default.</p>}
      <div className="flex flex-col gap-1.5">
        <label htmlFor="agent-model-discovery" className="text-xs font-medium">Discovery command (optional override)</label>
        <Input id="agent-model-discovery" value={discoveryCommand} onChange={(event) => onDiscoveryCommandChange(event.target.value)} className="font-mono text-xs" placeholder="my-agent models" />
        <p className="text-xs text-muted-foreground">Return a JSON array of model IDs or one ID per line. Runs directly without a shell. Leave empty for built-in discovery; commands with launch arguments need an override.</p>
      </div>
      <div><Button type="button" variant="outline" disabled={discovering || !command.trim() || (engine === "custom" && !discoveryCommand.trim())} onClick={discover}>{discovering ? "Discovering…" : "Discover models"}</Button></div>
      {error && discoverySource === source && <p role="alert" className="text-xs text-destructive">{error}</p>}
      {discovered && discoverySource === source && <div aria-live="polite" className="flex flex-col gap-2">
        <p className="text-xs text-muted-foreground">{available.length ? "Suggestions — add models to your list:" : "All discovered models are already in your list."}</p>
        {available.length > 0 && <Input aria-label="Filter suggestions" value={filter} onChange={(event) => setFilter(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter") event.preventDefault(); }} placeholder="Filter suggestions" />}
        <ul aria-label="Suggested models" className="max-h-48 overflow-y-auto space-y-1">
          {filtered.map((model) => <li key={model} className="flex items-center justify-between gap-2 rounded border px-2 py-1">
            <span className="break-all font-mono text-xs">{model}</span>
            <Button type="button" size="sm" variant="outline" aria-label={`Add ${model}`} onClick={() => add(model)}>Add</Button>
          </li>)}
        </ul>
      </div>}
    </fieldset>
  );
}
