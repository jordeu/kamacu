import { useEffect, useState } from "react";
import {
  useCleanEligible,
  type CleanEligiblePreview,
  type CleanEligibleResult,
  type EligibleItem,
} from "@/api/worktreeCleanup";
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
import { Skeleton } from "@/components/ui/skeleton";

interface CleanEligibleDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
}

/** Why a worktree is in the eligible set — maps the server reason to UI copy. */
const REASON_LABEL: Record<EligibleItem["reason"], string> = {
  orphaned: "Orphaned",
  done: "Done",
  pr_merged: "PR merged",
  pr_closed: "PR closed",
};

const REASSURANCE = `Only worktrees with no running session, no uncommitted changes, no unpushed commits, and no stash are removed. Nothing is forced; anything blocked or with unsaved work is skipped.`;

/** True when a settled value carried the applied {removed, skipped} shape (the
 *  confirm run) rather than the {items} preview (the dry run). */
function isResult(
  v: CleanEligiblePreview | CleanEligibleResult | undefined,
): v is CleanEligibleResult {
  return v != null && "removed" in v;
}

/**
 * The WTREE-03 / D-05 bulk clean-eligible dialog. On open it fetches the EXACT
 * server-computed safe set (a dry-run preview from fresh server state, never the
 * list cache); on confirm it removes that set. By construction it only ever
 * touches provably-safe worktrees (no session / no dirty / no unpushed / no
 * stash), so the CTA is the DEFAULT variant — nothing destructive is at stake.
 *
 * The body is mounted only while open, so the preview fetch fires once per open
 * and its local state resets on close (fresh state = a fresh mount) without a
 * setState-in-effect.
 */
export function CleanEligibleDialog({
  open,
  onOpenChange,
}: CleanEligibleDialogProps) {
  return (
    <AlertDialog open={open} onOpenChange={onOpenChange}>
      <AlertDialogContent>
        {open && <CleanEligibleBody />}
      </AlertDialogContent>
    </AlertDialog>
  );
}

function CleanEligibleBody() {
  const clean = useCleanEligible();
  // Snapshot of the preview set, captured once from the dry-run response so the
  // later confirm-run's {removed,skipped} result doesn't blank the list.
  const [items, setItems] = useState<EligibleItem[]>([]);
  const [summary, setSummary] = useState<CleanEligibleResult | null>(null);

  // Fire the dry-run preview once on mount. The effect only CALLS the mutation
  // (its result is captured in onSuccess) — no synchronous setState here.
  const previewMutate = clean.mutate;
  useEffect(() => {
    previewMutate(
      { dryRun: true },
      {
        onSuccess: (res) => {
          if (!isResult(res)) setItems(res.items);
        },
      },
    );
  }, [previewMutate]);

  const loadingPreview = clean.isPending && summary === null && items.length === 0;
  const count = items.length;

  function handleConfirm() {
    if (clean.isPending || count === 0) return;
    clean.mutate(
      { dryRun: false },
      {
        onSuccess: (res) => {
          if (isResult(res)) setSummary(res);
        },
      },
    );
  }

  const ctaLabel =
    clean.isPending && summary === null && items.length > 0
      ? `Cleaning up…`
      : `Remove ${count} worktrees`;

  return (
    <>
      <AlertDialogHeader>
        <AlertDialogTitle>{`Clean eligible worktrees?`}</AlertDialogTitle>
        <AlertDialogDescription asChild>
          <div className="space-y-3 text-left">
            <p>{REASSURANCE}</p>
            {loadingPreview ? (
              <div className="space-y-2">
                <Skeleton className="h-4 w-full" />
                <Skeleton className="h-4 w-full" />
                <Skeleton className="h-4 w-3/4" />
              </div>
            ) : count === 0 ? (
              <p className="text-sm text-muted-foreground">
                {`Nothing is eligible right now.`}
              </p>
            ) : (
              <ul className="max-h-56 space-y-1 overflow-y-auto">
                {items.map((item) => (
                  <li
                    key={`${item.repo}:${item.path}`}
                    className="text-xs text-muted-foreground"
                  >
                    <span>{item.project_name}</span>
                    {` · `}
                    <span className="font-mono">{item.path}</span>
                    {` · `}
                    <span>{REASON_LABEL[item.reason]}</span>
                  </li>
                ))}
              </ul>
            )}
            {summary && (
              <p className="text-xs text-muted-foreground">
                {`Removed ${summary.removed}; ${summary.skipped} skipped.`}
              </p>
            )}
          </div>
        </AlertDialogDescription>
      </AlertDialogHeader>

      <AlertDialogFooter>
        <AlertDialogCancel>{`Cancel`}</AlertDialogCancel>
        <AlertDialogAction
          disabled={clean.isPending || count === 0 || summary !== null}
          onClick={(e) => {
            // Keep the dialog open until the bulk run lands (pending, summary).
            e.preventDefault();
            handleConfirm();
          }}
        >
          {ctaLabel}
        </AlertDialogAction>
      </AlertDialogFooter>
    </>
  );
}
