/**
 * Per-file commit-state markers for the Diff tab: an amber filled dot marks an
 * uncommitted file (staged/unstaged/untracked — content not fully captured in
 * commits, the `git status` signal); a muted hollow ring marks an unpushed file
 * (committed content missing from the remote). Both may appear at once (commit
 * half, keep editing). Rendered before the path in the diff header and the
 * file-tree leaf. Decorative by design — the totals bar's
 * "· N uncommitted · N unpushed" summary is the legend and the accessible text.
 */
export function CommitStateMarkers({
  uncommitted,
  unpushed,
}: {
  uncommitted: boolean;
  unpushed: boolean;
}) {
  if (!uncommitted && !unpushed) return null;
  return (
    <span className="flex shrink-0 items-center gap-1" aria-hidden="true">
      {uncommitted && (
        <span
          title="Uncommitted changes"
          className="size-1.5 rounded-full bg-amber-400"
        />
      )}
      {unpushed && (
        <span
          title="Not pushed yet"
          className="size-2 rounded-full border border-muted-foreground/60"
        />
      )}
    </span>
  );
}
