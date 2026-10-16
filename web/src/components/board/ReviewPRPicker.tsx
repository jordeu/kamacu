import { useRef, useState } from "react";
import { useNavigate } from "react-router";
import { Popover } from "radix-ui";
import { GitPullRequest, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useOpenPullRequests, useOpenReview } from "@/api/pullRequests";

export function ReviewPRPicker({ projectId }: { projectId: number }) {
  const searchInput = useRef<HTMLInputElement>(null);
  const [visible, setVisible] = useState(false);
  const [search, setSearch] = useState("");
  const navigate = useNavigate();
  const list = useOpenPullRequests(projectId, visible);
  const review = useOpenReview(projectId);
  const query = search.trim();
  const number = /^#?[1-9]\d*$/.test(query) ? Number(query.replace(/^#/, "")) : null;
  const validNumber = number !== null && Number.isSafeInteger(number);
  const prs = (list.data?.prs ?? []).filter((pr) =>
    `${pr.number} ${pr.title} ${pr.author}`.toLowerCase().includes(query.replace(/^#/, "").toLowerCase()),
  );
  const open = (prNumber: number) => {
    if (review.isPending) return;
    review.mutate(prNumber, {
      onSuccess: (res) => {
        setVisible(false);
        navigate(`/projects/${projectId}/tasks/${res.task.id}`);
      },
    });
  };
  const listFailed = list.isError || (list.data && list.data.state !== "ok");

  return (
    <Popover.Root open={visible} onOpenChange={(next) => {
      if (review.isPending) return;
      setVisible(next);
      if (next) { setSearch(""); review.reset(); }
    }}>
      <Popover.Trigger asChild>
        <Button variant="ghost" size="icon-sm" aria-label="Open a PR review" title="Open a PR review" className="text-muted-foreground">
          <GitPullRequest className="size-3.5" />
        </Button>
      </Popover.Trigger>
      <Popover.Portal>
        <Popover.Content onOpenAutoFocus={(event) => { event.preventDefault(); searchInput.current?.focus(); }} align="end" sideOffset={8} aria-label="Open a PR review" className="z-50 w-80 max-w-[calc(100vw-2rem)] rounded-md border border-border bg-popover p-3 text-popover-foreground shadow-md">
          <div className="mb-2 flex items-center justify-between gap-2">
            <h2 className="text-sm font-medium">Open a PR review</h2>
            <Popover.Close asChild>
              <Button variant="ghost" size="icon-sm" aria-label="Close PR picker" disabled={review.isPending}><X className="size-3.5" /></Button>
            </Popover.Close>
          </div>
          <form onSubmit={(e) => { e.preventDefault(); if (validNumber) open(number!); }} className="space-y-2">
            <Input ref={searchInput} aria-label="Search open PRs or enter a PR number" placeholder="Search or enter PR number…" value={search} disabled={review.isPending} onChange={(e) => { setSearch(e.target.value); review.reset(); }} />
            <p className="text-xs text-muted-foreground">Enter any PR number, including closed or merged PRs.</p>
            {validNumber && <Button type="submit" size="sm" className="w-full" disabled={review.isPending}>Open review for #{number}</Button>}
          </form>
          {review.isPending && <p role="status" className="mt-2 text-xs text-muted-foreground">Setting up the review worktree…</p>}
          {review.isError && <p role="alert" className="mt-2 text-xs text-destructive">Couldn't open this review. Check the PR number and try again.</p>}
          <div className="mt-3 border-t border-border pt-2">
            <p className="mb-1 text-xs font-medium text-muted-foreground">Open pull requests</p>
            {list.isLoading && <p role="status" className="py-2 text-xs text-muted-foreground">Loading open PRs…</p>}
            {listFailed && <div className="py-2 text-xs text-muted-foreground">Couldn't load open PRs. <button type="button" className="underline underline-offset-2" onClick={() => list.refetch()} disabled={list.isFetching}>Try again</button></div>}
            {list.data?.state === "ok" && prs.length === 0 && <p className="py-2 text-xs text-muted-foreground">{query ? "No matching open PRs. Enter a number to open any PR." : "No open pull requests."}</p>}
            <div className="max-h-64 overflow-y-auto" aria-busy={review.isPending}>
              {prs.map((pr) => <button key={pr.number} type="button" disabled={review.isPending} onClick={() => open(pr.number)} className="block w-full rounded-sm px-2 py-2 text-left hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50">
                <span className="block break-words text-sm">{pr.title}</span>
                <span className="block text-xs text-muted-foreground">#{pr.number} · @{pr.author}</span>
              </button>)}
            </div>
          </div>
        </Popover.Content>
      </Popover.Portal>
    </Popover.Root>
  );
}
