import { useState } from "react";
import { useNavigate } from "react-router";
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { useAgents, useProjects } from "@/api/queries";
import { useCreateTask } from "@/api/mutations";

interface NewTaskDialogProps {
  projectId: number;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/**
 * Full create dialog (D-08): title + raw markdown description.
 * No preview here — preview lives in the task view (D-10).
 */
export function NewTaskDialog({
  projectId,
  open,
  onOpenChange,
}: NewTaskDialogProps) {
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [agentId, setAgentId] = useState<number | null>(null);
  const [modelSelection, setModelSelection] = useState<{ agentId: number; model: string } | null>(null);
  const { data: agents } = useAgents();
  const { data: projects } = useProjects();
  const projectAgentId = projects?.find((project) => project.id === projectId)?.agent_id;
  const projectAgent = agents?.find((agent) => agent.id === projectAgentId);
  const selectedAgentId = agentId ?? projectAgentId;
  const selectedAgent = agents?.find((agent) => agent.id === selectedAgentId);
  // A background refresh can change the project default or its curated list.
  const model = modelSelection && selectedAgent?.model_argument && modelSelection.agentId === selectedAgentId && selectedAgent.models?.includes(modelSelection.model)
    ? modelSelection.model : "";
  const navigate = useNavigate();
  const createTask = useCreateTask(projectId);

  function handleOpenChange(nextOpen: boolean) {
    if (!nextOpen) {
      setAgentId(null);
      setModelSelection(null);
    }
    onOpenChange(nextOpen);
  }

  function handleSubmit(startAgent = false) {
    const trimmed = title.trim();
    if (trimmed === "" || !selectedAgent || createTask.isPending) return;
    createTask.mutate(
      { title: trimmed, description, agent_id: selectedAgentId, model },
      {
        onSuccess: (task) => {
          setTitle("");
          setDescription("");
          setAgentId(null);
          setModelSelection(null);
          handleOpenChange(false);
          if (startAgent) {
            navigate(`/projects/${projectId}/tasks/${task.id}`, { state: { startAgent: true } });
          }
        },
      },
    );
  }

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
      <DialogContent className="sm:max-w-[560px]">
        <DialogHeader>
          <DialogTitle>New task</DialogTitle>
        </DialogHeader>
        <div className="flex flex-col gap-4">
          <Input
            autoFocus
            value={title}
            placeholder="Task title"
            onChange={(e) => setTitle(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && !e.nativeEvent.isComposing) handleSubmit(true);
            }}
          />
          <Textarea
            value={description}
            placeholder="Description (markdown)"
            className="min-h-[160px]"
            onChange={(e) => setDescription(e.target.value)}
          />
          <div className="grid grid-cols-2 gap-4">
            <div className="flex flex-col gap-1.5">
              <label htmlFor="task-agent" className="text-xs font-medium">Agent</label>
              <select id="task-agent" className="h-9 rounded-md border bg-background px-2 text-sm" value={agentId ?? ""}
                onChange={(event) => { setAgentId(event.target.value === "" ? null : Number(event.target.value)); setModelSelection(null); }}>
                <option value="">Project default{projectAgent ? ` (${projectAgent.name})` : ""}</option>
                {agents?.map((agent) => <option key={agent.id} value={agent.id}>{agent.name}</option>)}
              </select>
            </div>
            <div className="flex flex-col gap-1.5">
              <label htmlFor="task-model" className="text-xs font-medium">Model</label>
              <select id="task-model" className="h-9 rounded-md border bg-background px-2 text-sm disabled:opacity-50" value={model}
                disabled={!selectedAgent?.model_argument || !selectedAgent.models?.length}
                onChange={(event) => setModelSelection(selectedAgentId == null ? null : { agentId: selectedAgentId, model: event.target.value })}>
                <option value="">Agent default</option>
                {selectedAgent?.models?.map((id) => <option key={id} value={id}>{id}</option>)}
              </select>
            </div>
          </div>
          {selectedAgent && (!selectedAgent.model_argument || !selectedAgent.models?.length) &&
            <p className="text-xs text-muted-foreground">Add models in Settings → Agents to choose a model for this agent.</p>}
        </div>
        {createTask.isError && (
          <p role="alert" className="text-sm text-destructive">
            Could not create the task. {createTask.error.message}
          </p>
        )}
        <DialogFooter>
          <Button variant="ghost" onClick={() => handleOpenChange(false)}>
            Cancel
          </Button>
          <Button
            variant="outline"
            onClick={() => handleSubmit()}
            disabled={title.trim() === "" || !selectedAgent || createTask.isPending}
          >
            Create task
          </Button>
          <Button
            onClick={() => handleSubmit(true)}
            disabled={title.trim() === "" || !selectedAgent || createTask.isPending}
          >
            Create &amp; Start
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
