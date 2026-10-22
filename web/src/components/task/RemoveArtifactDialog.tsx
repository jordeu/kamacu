import type { Artifact } from "@/api/artifacts";
import { useRemoveArtifact } from "@/api/artifacts";
import {
  AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent,
  AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle,
} from "@/components/ui/alert-dialog";

export function RemoveArtifactDialog({ artifact, taskId, onClose }: {
  artifact: Artifact | null; taskId: number; onClose: () => void;
}) {
  const remove = useRemoveArtifact(taskId);
  return (
    <AlertDialog open={artifact !== null} onOpenChange={(open) => { if (!open) { remove.reset(); onClose(); } }}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Remove artifact?</AlertDialogTitle>
          <AlertDialogDescription>
            “{artifact?.title}” and its uploaded files will be permanently removed. This can’t be undone.
          </AlertDialogDescription>
        </AlertDialogHeader>
        {remove.error && <p role="alert" className="text-sm text-destructive">{remove.error.message}</p>}
        <AlertDialogFooter>
          <AlertDialogCancel disabled={remove.isPending}>Cancel</AlertDialogCancel>
          <AlertDialogAction variant="destructive" disabled={remove.isPending} onClick={(event) => {
            event.preventDefault();
            if (artifact) remove.mutate(artifact.id, { onSuccess: () => { remove.reset(); onClose(); } });
          }}>
            {remove.isPending ? "Removing…" : "Remove artifact"}
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
