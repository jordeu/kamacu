import { useState, type FormEvent } from "react";
import { ApiError } from "@/api/client";
import { useCreateAgent, useUpdateAgent } from "@/api/mutations";
import type { Agent } from "@/api/types";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { AgentModelsEditor } from "./AgentModelsEditor";
import { Input } from "@/components/ui/input";

export interface AgentNameDialogProps {
  /** "create" starts empty; "edit" prefills from `agent`. */
  mode: "create" | "edit";
  /** Required in "edit" mode — the agent being edited. */
  agent?: Agent;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/** Uppercase the first letter so server messages render in sentence case. */
function sentenceCase(message: string): string {
  return message.charAt(0).toUpperCase() + message.slice(1);
}

/** Mount a fresh form on each open so cancel discards all draft configuration. */
export function AgentNameDialog(props: AgentNameDialogProps) {
  if (!props.open) return null;
  return <AgentNameForm key={`${props.mode}:${props.agent?.id ?? "new"}`} {...props} />;
}

function AgentNameForm({
  mode,
  agent,
  open,
  onOpenChange,
}: AgentNameDialogProps) {
  const [name, setName] = useState(mode === "edit" ? (agent?.name ?? "") : "");
  const [command, setCommand] = useState(
    mode === "edit" ? (agent?.command ?? "") : "",
  );
  // M001 gate follow-up: claude-engine extra params (relocated from the global
  // setting). Only shown for claude-engine agents (the system seed).
  const [extraParams, setExtraParams] = useState(
    mode === "edit" ? (agent?.extra_params ?? "") : "",
  );
  const [models, setModels] = useState<string[]>(agent?.models ?? []);
  const [modelArgument, setModelArgument] = useState(agent?.model_argument ?? "");
  const [discoveryCommand, setDiscoveryCommand] = useState(agent?.model_discovery_command ?? "");
  const [error, setError] = useState<string | null>(null);
  const createAgent = useCreateAgent();
  const updateAgent = useUpdateAgent();

  const pending = mode === "create" ? createAgent.isPending : updateAgent.isPending;

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    try {
      if (mode === "create") {
        await createAgent.mutateAsync({ name: name.trim(), command: command.trim(), models, model_argument: modelArgument, model_discovery_command: discoveryCommand });
      } else {
        await updateAgent.mutateAsync({
          id: agent!.id,
          name: name.trim(),
          command: command.trim(),
          models,
          model_argument: modelArgument,
          model_discovery_command: discoveryCommand,
          // Only the claude engine uses extra_params; send it when editing a
          // claude agent so the field is persisted on the row.
          ...(agent!.engine === "claude" ? { extra_params: extraParams } : {}),
        });
      }
      onOpenChange(false);
    } catch (err) {
      setError(
        err instanceof ApiError
          ? sentenceCase(err.message)
          : "Couldn't save. Try again.",
      );
    }
  }

  const title = mode === "create" ? "New coding agent" : `Configure ${agent?.name ?? "agent"}`;
  const submitLabel = mode === "create" ? "Create" : "Save";
  const trimmed = name.trim() === "" || command.trim() === "";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent aria-describedby={undefined} className="max-h-[90vh] overflow-y-auto sm:max-w-[600px]">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
        </DialogHeader>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4">
          <div className="flex flex-col gap-1.5">
            <label htmlFor="agent-name" className="text-xs font-medium">
              Name
            </label>
            <Input
              id="agent-name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              autoFocus
            />
          </div>
          <div className="flex flex-col gap-1.5">
            <label htmlFor="agent-command" className="text-xs font-medium">
              Launch command
            </label>
            <Input
              id="agent-command"
              value={command}
              onChange={(event) => setCommand(event.target.value)}
              className="font-mono text-xs"
              placeholder="e.g. gemini, or aider --model sonnet"
            />
            <p className="text-xs text-muted-foreground">
              {"Runs in the task worktree. Use "}
              <code className="font-mono">{"{{worktree}}"}</code>
              {" or "}
              <code className="font-mono">{"{{session_id}}"}</code>
              {" to pass the path or an id."}
            </p>
          </div>
          {mode === "edit" && agent?.engine === "claude" && (
            <details className="rounded-md border px-3 py-2">
              <summary className="cursor-pointer text-sm font-medium">Advanced launch settings</summary>
              <div className="mt-3 flex flex-col gap-1.5">
                <label htmlFor="agent-extra-params" className="text-xs font-medium">
                  Extra parameters
                </label>
                <Input
                  id="agent-extra-params"
                  value={extraParams}
                  onChange={(event) => setExtraParams(event.target.value)}
                  className="font-mono text-xs"
                  placeholder="--dangerously-skip-permissions"
                />
                <p className="text-xs text-muted-foreground">
                  {"Extra flags appended to every claude spawn. Clear to restore permission prompts."}
                </p>
              </div>
            </details>
          )}
          {open && <AgentModelsEditor
            engine={mode === "edit" ? (agent?.engine ?? "custom") : "custom"}
            command={command}
            models={models}
            onModelsChange={setModels}
            modelArgument={modelArgument}
            onModelArgumentChange={setModelArgument}
            discoveryCommand={discoveryCommand}
            onDiscoveryCommandChange={setDiscoveryCommand}
          />}
          {error && <p role="alert" className="text-xs text-destructive">{error}</p>}
          <DialogFooter>
            <Button
              type="button"
              variant="ghost"
              onClick={() => onOpenChange(false)}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={pending || trimmed}>
              {submitLabel}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
