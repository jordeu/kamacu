import { useNavigate } from "react-router";
import { useDeleteTask } from "@/api/mutations";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog";

interface DeleteTaskDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskId: number;
  taskTitle: string;
  projectId: number;
}

/** Hard delete behind a confirmation dialog (D-11). */
export function DeleteTaskDialog({
  open,
  onOpenChange,
  taskId,
  taskTitle,
  projectId,
}: DeleteTaskDialogProps) {
  const deleteTask = useDeleteTask(projectId);
  const navigate = useNavigate();

  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Delete task?</AlertDialogTitle>
          <AlertDialogDescription>
            {`"${taskTitle}" will be permanently deleted. This can't be undone.`}
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel>Cancel</AlertDialogCancel>
          <AlertDialogAction
            variant="destructive"
            disabled={deleteTask.isPending}
            onClick={(e) => {
              // Keep the dialog open until the delete lands, then go back
              // to the board.
              e.preventDefault();
              deleteTask
                .mutateAsync(taskId)
                .then(() => {
                  onOpenChange(false);
                  navigate(`/projects/${projectId}`);
                })
                .catch(() => {
                  // Delete failed — leave the dialog open so the user can
                  // retry or cancel.
                });
            }}
          >
            Delete task
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
