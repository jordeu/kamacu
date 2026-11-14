import { useState } from "react";
import { Link } from "react-router";
import { ArrowUpRight, GitPullRequest } from "lucide-react";
import { useCatchUpContext, useCatchUpPRDetail, useCatchUpPRStatus, useMessageHistory, type CatchUpPR } from "@/api/catchUp";
import type { FeedbackQuestion } from "@/api/inbox";
import type { Project } from "@/api/types";
import { STATUS_LABELS } from "@/api/types";
import { Button } from "@/components/ui/button";
import { ReadOnlyDescription } from "@/components/task/DescriptionTab";
import { PRLifecycleIcon, PRStatusDetails } from "@/components/task/PRStatusOverview";
import { InboxText } from "./InboxText";

export const contextProse = "prose prose-invert prose-sm max-w-none min-w-0 break-words prose-headings:text-sm prose-a:text-blue-400 prose-pre:max-w-full prose-pre:overflow-x-auto [overflow-wrap:anywhere]";
export const contextCard = "min-w-0 rounded-xl border border-border bg-card/50 p-4 sm:p-5";
export function CatchUpReferences({ names, prs, scratchpad }: { names: string[]; prs: CatchUpPR[]; scratchpad: boolean }) {
  return <nav aria-label="Message project and PR references" className="mb-5 flex flex-wrap gap-2 text-xs">
    {(names.length ? names : scratchpad ? ["Scratchpad"] : ["Notification"]).map(name => <span key={name} className="rounded-md border border-emerald-400/20 bg-emerald-400/5 px-2 py-1 text-emerald-200">{name}</span>)}
    {prs.map(pr => <a key={pr.url} href={pr.url} target="_blank" rel="noopener noreferrer" className="inline-flex min-w-0 items-center gap-1 rounded-md border border-orange-300/20 bg-orange-300/5 px-2 py-1 text-orange-200 hover:underline"><GitPullRequest className="size-3 shrink-0" /><span className="break-all">{pr.repository} · PR {pr.number}</span></a>)}
  </nav>;
}
function HistoryEvent({ question: q, current = false }: { question: FeedbackQuestion; current?: boolean }) {
  return <article className={`relative min-w-0 border-l pb-6 pl-5 ${current ? "border-orange-300/50" : "border-border"}`}>
    <span aria-hidden="true" className={`absolute -left-1 top-1.5 size-2 rounded-full ${current ? "bg-orange-300" : "bg-muted-foreground"}`} />
    <p className="mb-3 text-[11px] text-muted-foreground"><time dateTime={new Date(q.created_at).toISOString()}>{new Date(q.created_at).toLocaleString()}</time> · {q.origin === "notification" ? "Notification" : "Agent report"}{current && " · This message"}</p>
    <div className={current ? "rounded-xl border border-orange-300/30 bg-orange-300/5 p-4 sm:p-5" : "space-y-3"}>
      <p className="mb-3 text-xs font-medium text-muted-foreground">{q.kind === "completion" ? "Work reported complete" : "Decision requested"}{q.task_id > 0 && ` · Task ${q.task_id}`}</p>
      {!current && q.kind !== "completion" && <h3 className="mb-2 text-sm font-medium"><InboxText text={q.question} /></h3>}
      <div className="whitespace-pre-wrap break-words text-sm leading-relaxed"><InboxText text={q.abstract || q.summary} /></div>
      <details className="mt-4"><summary className="cursor-pointer text-xs text-muted-foreground">Read original context</summary><div className={`mt-4 ${contextProse}`}><InboxText text={q.context} paragraphs /></div>
        {!current && q.answer && <div className="mt-4 border-t border-border pt-3 text-xs"><p className="mb-2 text-muted-foreground">Recorded answer · {q.status}</p><InboxText text={q.answer} /></div>}
      </details>
    </div>
  </article>;
}
export function CatchUpHistory({ question }: { question: FeedbackQuestion }) {
  const history = useMessageHistory(question.id);
  const messages = history.data?.pages.flatMap(page => page.items) ?? [];
  return <section aria-labelledby="catch-up-history-title" className="min-w-0">
    <div className="mb-5 flex flex-wrap items-center justify-between gap-2"><h2 id="catch-up-history-title" className="text-lg font-semibold">How we got here</h2><span className="text-xs text-muted-foreground">Newest first</span></div>
    <div className="ml-2"><HistoryEvent question={question} current />
      {messages.slice(0, 3).map(q => <HistoryEvent key={q.id} question={q} />)}
      {messages.length > 3 && <details><summary className="mb-5 cursor-pointer rounded-lg border border-dashed border-border p-3 text-xs text-muted-foreground">Show {messages.length - 3} earlier messages</summary>{messages.slice(3).map(q => <HistoryEvent key={q.id} question={q} />)}</details>}
    </div>
    {history.isPending && <p role="status" className="text-sm text-muted-foreground">Loading earlier messages…</p>}
    {history.isError && <div role="alert" className="text-sm">Couldn’t load earlier messages. <Button variant="ghost" size="sm" onClick={() => void history.refetch()}>Retry history</Button></div>}
    {history.isSuccess && !messages.length && <p className="text-xs text-muted-foreground">No earlier messages are recorded for the related tasks.</p>}
    {history.hasNextPage && <Button variant="outline" size="sm" disabled={history.isFetchingNextPage} onClick={() => void history.fetchNextPage()}>{history.isFetchingNextPage ? "Loading…" : "Load older history"}</Button>}
  </section>;
}
export function CatchUpTasks({ context, onNavigate }: { context: ReturnType<typeof useCatchUpContext>; onNavigate: () => void }) {
  return <section className="min-w-0 space-y-4" aria-label="Related tasks">
    <h2 className="text-lg font-semibold">The related work</h2><p className="text-xs text-muted-foreground">Current task state, separate from the historical messages.</p>
    {context.queries.map((query, index) => query.isError ? <div key={index} role="alert" className={contextCard}>Couldn’t load a related task. <Button size="sm" variant="ghost" onClick={() => void query.refetch()}>Retry task</Button></div> : query.isPending ? <p key={index} role="status" className="text-sm text-muted-foreground">Loading task…</p> : null)}
    {context.tasks.map(task => <article key={task.id} className={contextCard}>
      <div className="flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground"><span>{context.projects.data?.find(p => p.id === task.project_id)?.name} / Task {task.id}</span><span>{STATUS_LABELS[task.status]}</span></div>
      <h3 className="mt-3 break-words text-sm font-semibold">{task.title}</h3>
      <Link onClick={onNavigate} to={`/projects/${task.project_id}/tasks/${task.id}`} className="mt-2 inline-flex items-center gap-1 text-xs text-blue-400 hover:underline">Open task <ArrowUpRight className="size-3" /></Link>
      <details className="mt-4"><summary className="cursor-pointer text-xs text-muted-foreground">Task brief{task.branch && " & branch"}</summary><div className={`mt-4 ${contextProse}`}><ReadOnlyDescription source={task.description} /></div>{task.branch && <p className="mt-4 break-all text-xs text-muted-foreground">Branch: {task.branch}</p>}</details>
    </article>)}
    {!context.queries.length && <p className="rounded-lg border border-dashed border-border p-4 text-sm text-muted-foreground">No task is linked to this message.</p>}
  </section>;
}
function KnownProjectPR({ pr, project, initiallyExpanded }: { pr: CatchUpPR; project: Project; initiallyExpanded: boolean }) {
  const [expanded, setExpanded] = useState(initiallyExpanded);
  const detail = useCatchUpPRDetail(project.id, pr, expanded);
  const status = useCatchUpPRStatus(project.id, pr, expanded);
  const snapshot = status.data && status.isError ? { ...status.data, stale: true, error: "Could not refresh PR status" } : status.data;
  const lifecycle = snapshot?.lifecycle && snapshot.lifecycle !== "unknown" ? snapshot.lifecycle : detail.data?.state.toLowerCase();
  return <details className={contextCard} open={expanded} onToggle={event => {
    if (event.target === event.currentTarget) setExpanded(event.currentTarget.open);
  }}>
    <summary className="cursor-pointer list-none space-y-2 [&::-webkit-details-marker]:hidden"><div className="flex flex-wrap items-center justify-between gap-2"><a href={pr.url} target="_blank" rel="noopener noreferrer" className="inline-flex items-center gap-2 text-xs text-blue-400 hover:underline"><PRLifecycleIcon state={lifecycle} />{pr.repository} · PR {pr.number}<ArrowUpRight className="size-3" /></a><span className="text-xs text-muted-foreground">{lifecycle}</span></div>
      <h3 className="break-words text-base font-semibold">{detail.data?.title ?? (detail.isFetching ? "Loading PR…" : expanded ? "PR details unavailable" : "Show PR details")}</h3>
      {detail.data && <p className="break-words text-xs leading-relaxed text-muted-foreground">@{detail.data.author} · {detail.data.commits} commits · <span className="break-all font-mono">{detail.data.headRefName}</span> → <span className="break-all font-mono">{detail.data.baseRefName}</span></p>}
      {detail.data && <p className="text-[11px] text-muted-foreground">Description updated: <time dateTime={new Date(detail.dataUpdatedAt).toISOString()}>{new Date(detail.dataUpdatedAt).toLocaleString()}</time></p>}
      <p className="text-xs text-muted-foreground">{expanded ? "Hide details ↑" : "Show details ↓"}</p>
    </summary>
    {expanded && <div className="mt-5 grid min-w-0 gap-6 lg:grid-cols-[minmax(0,1fr)_20rem]">
      <div className="min-w-0">{detail.isError && <div role="alert" className="mb-4 text-sm">Couldn’t refresh the PR description. <Button variant="ghost" size="sm" onClick={() => void detail.refetch()}>Retry PR details</Button></div>}{detail.data && <div className={contextProse}><ReadOnlyDescription source={detail.data.body} /></div>}</div>
      <aside className="min-w-0 self-start rounded-lg border border-border bg-muted/30 p-4" aria-label={`${pr.repository} PR ${pr.number} status overview`}>
        {status.isPending && <p role="status" className="mb-3 text-xs text-muted-foreground">Loading PR status…</p>}
        <PRStatusDetails snapshot={snapshot} repository={pr.repository} number={pr.number} showJobs />
        {(status.isError || snapshot?.error) && <p role="alert" className="mt-3 text-xs">Couldn’t refresh PR status. <Button size="sm" variant="ghost" disabled={status.isFetching} onClick={() => void status.refetch()}>Retry PR status</Button></p>}
      </aside>
    </div>}
  </details>;
}
export function CatchUpPRs({ context }: { context: ReturnType<typeof useCatchUpContext> }) {
  return <section aria-label="Related pull requests" className="mt-8 space-y-4 border-t border-border pt-7"><h2 className="text-lg font-semibold">Pull requests · full context</h2><p className="text-xs text-muted-foreground">Current PR descriptions, CI and reviews, separate from the historical story.</p>
    {context.projects.isError && <p role="alert">Couldn’t load project references. <Button variant="ghost" onClick={() => void context.projects.refetch()}>Retry projects</Button></p>}
    {context.prs.map((pr, index) => {
      const project = context.projects.data?.find(p => p.github_repo?.toLowerCase() === pr.repository.toLowerCase());
      return project ? <KnownProjectPR key={pr.url} pr={pr} project={project} initiallyExpanded={index === 0} /> : <article key={pr.url} className={contextCard}><a href={pr.url} target="_blank" rel="noopener noreferrer" className="text-sm text-blue-400 hover:underline">{pr.repository} · PR {pr.number}</a><p className="mt-3 text-xs text-muted-foreground">{context.projects.isPending ? "Loading project reference…" : "No Kamacu project is linked to this repository. Open the PR on GitHub for its details."}</p></article>;
    })}
    {!context.prs.length && <p className="text-sm text-muted-foreground">No pull requests are linked or explicitly referenced in this message.</p>}
  </section>;
}
