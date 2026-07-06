import { useEffect, useState, type FormEvent } from "react";
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

/**
 * Create/edit agent dialog (M001) — a two-field variant of WorkspaceNameDialog.
 * One dialog serves both flows: "create" opens with empty fields, "edit"
 * prefills the existing name + command. The server's duplicate-name reject
 * (`name UNIQUE COLLATE NOCASE`, migration 00013) arrives as `ApiError.message`
 * and renders sentence-cased inline; Save is disabled while pending or empty.
 *
 * The Command field is mono and carries placeholder help for the
 * {{worktree}} / {{session_id}} template tokens (D-M001-1). A system agent's
 * engine is server-locked; the form edits only name + command for any agent.
 */
export function AgentNameDialog({
  mode,
  agent,
  open,
  onOpenChange,
}: AgentNameDialogProps) {
  const [name, setName] = useState(mode === "edit" ? (agent?.name ?? "") : "");
  const [command, setCommand] = useState(
    mode === "edit" ? (agent?.command ?? "") : "",
  );
  const [error, setError] = useState<string | null>(null);
  const createAgent = useCreateAgent();
  const updateAgent = useUpdateAgent();

  const pending = mode === "create" ? createAgent.isPending : updateAgent.isPending;

  // Prefill each time the dialog opens; clear any prior error.
  useEffect(() => {
    if (open) {
      setName(mode === "edit" ? (agent?.name ?? "") : "");
      setCommand(mode === "edit" ? (agent?.command ?? "") : "");
      setError(null);
    }
  }, [open, mode, agent?.name, agent?.command]);

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(null);
    try {
      if (mode === "create") {
        await createAgent.mutateAsync({ name: name.trim(), command: command.trim() });
      } else {
        await updateAgent.mutateAsync({
          id: agent!.id,
          name: name.trim(),
          command: command.trim(),
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

  const title = mode === "create" ? "New agent" : "Edit agent";
  const submitLabel = mode === "create" ? "Create" : "Save";
  const trimmed = name.trim() === "" || command.trim() === "";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent aria-describedby={undefined}>
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
              Command
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
          {error && <p className="text-xs text-destructive">{error}</p>}
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
