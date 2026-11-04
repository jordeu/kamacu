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
  const navigate = useNavigate();
  const createTask = useCreateTask(projectId);

  function handleSubmit(startAgent = false) {
    const trimmed = title.trim();
    if (trimmed === "" || createTask.isPending) return;
    createTask.mutate(
      { title: trimmed, description },
      {
        onSuccess: (task) => {
          setTitle("");
          setDescription("");
          onOpenChange(false);
          if (startAgent) {
            navigate(`/projects/${projectId}/tasks/${task.id}`, { state: { startAgent: true } });
          }
        },
      },
    );
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
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
        </div>
        {createTask.isError && (
          <p role="alert" className="text-sm text-destructive">
            Could not create the task. {createTask.error.message}
          </p>
        )}
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            variant="outline"
            onClick={() => handleSubmit()}
            disabled={title.trim() === "" || createTask.isPending}
          >
            Create task
          </Button>
          <Button
            onClick={() => handleSubmit(true)}
            disabled={title.trim() === "" || createTask.isPending}
          >
            Create &amp; Start
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
