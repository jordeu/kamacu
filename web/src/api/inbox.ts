import { useEffect, useRef } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { get, post } from "./client";

export interface FeedbackQuestion {
 origin?: string;
 hidden?: boolean;
 delivery_mode?: string;
 delivery_status?: string;
  kind?: "question" | "completion";
  id: string;
  task_id: number;
  project_id: number;
  task_title: string;
  project_name: string;
  session_id: string;
  abstract: string;
  summary: string;
  context: string;
  question: string;
  options: { id: string; label: string }[];
  status: "pending" | "answered" | "dismissed" | "unanswered";
  reason: string;
  answer: string;
  answer_source: string;
  revision: number;
  created_at: number;
  updated_at: number;
  acknowledged_at: number | null;
}
interface InboxPage {
  items: FeedbackQuestion[];
  pending_count: number;
  has_more: boolean;
}
export interface FeedbackResult { outcome: string; question: FeedbackQuestion }
export function useInbox(view: "pending" | "archived" = "pending", offset = 0) {
  return useQuery({
    queryKey: ["inbox", view, offset],
    queryFn: () => get<InboxPage>(`/api/inbox?view=${view}&offset=${offset}`),
    refetchInterval: 3000,
    refetchIntervalInBackground: true,
  });
}
export function useInboxMessage(id: string | null) {
  return useQuery({
    queryKey: ["inbox", "message", id],
    queryFn: () => get<FeedbackQuestion>(`/api/inbox/${encodeURIComponent(id!)}`),
    enabled: !!id,
    refetchInterval: 3000,
  });
}
export function useFeedbackAction() {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ question, action, answer, option_id }: {
      question: FeedbackQuestion; action: "answer" | "dismiss"; answer?: string; option_id?: string;
    }) => post<FeedbackResult>(`/api/inbox/${question.id}/${action}`, { revision: question.revision, answer, option_id }),
    onSuccess: (result) => {
      void client.invalidateQueries({ queryKey: ["inbox"] });
      if (result.outcome === "accepted" && completionClosesSession(result.question)) {
        for (const key of ["tasks", "task", "sessions", "agent-statuses"]) void client.invalidateQueries({ queryKey: [key] });
      }
    },
  });
}
const completionLabels: Record<string, string> = {
  move_to_review: "Move to review and close session",
  move_to_done: "Move to done and close session",
};
export function completionActions(q: FeedbackQuestion) {
  if (q.kind !== "completion" || q.task_id === 0) return [];
  return Object.entries(completionLabels)
    .filter(([id]) => q.options.some(option => option.id === id))
    .map(([id, label]) => ({ id, label }));
}
export function completionClosesSession(q: FeedbackQuestion) {
  return q.kind === "completion" && (q.reason === "exit" || Object.hasOwn(completionLabels, q.reason));
}
export function feedbackTaskURL(q: FeedbackQuestion) {
  const path = q.task_id === 0 ? "/global" : `/projects/${q.project_id}/tasks/${q.task_id}`;
  return `${path}?feedback_session=${encodeURIComponent(q.session_id)}`;
}
export function feedbackOutcomeMessage(result: FeedbackResult) {
  if (result.outcome === "stale_revision") return "This question was reopened. Review its current version before answering.";
  if (result.outcome === "already_resolved") {
    if (result.question.status === "answered") return `This question was already answered: ${result.question.answer}`;
    return "This question is no longer waiting for an answer. You can find it in Archived.";
  }
  if (result.question.kind === "completion" && result.question.status === "answered") {
    if (result.question.reason === "move_to_review") return "Task moved to review. Message archived and agent session is closing.";
    if (result.question.reason === "move_to_done") return "Task moved to done. Message archived and agent session is closing.";
    if (result.question.reason === "exit") return "Message archived. Agent session is closing.";
    if (result.question.reason === "seen") return "Marked as seen. Agent session stays open.";
    return "Next prompt sent.";
  }
  return result.question.status === "dismissed" ? "Message dismissed and archived." : "Answer sent. The question has been archived.";
}

// The server snapshots all pending completions for this task in one read.
// Revision checks preserve messages reopened after that snapshot.
export async function dismissTaskCompletions(taskId: number, signal: AbortSignal) {
  const completions = await get<FeedbackQuestion[]>(`/api/inbox/tasks/${taskId}/completions`, { signal });
  for (const question of completions) {
    signal.throwIfAborted();
    await post<FeedbackResult>(`/api/inbox/${question.id}/dismiss`, { revision: question.revision }, { signal });
  }
}

export function useDismissTaskCompletions(taskId: number | undefined) {
  const client = useQueryClient();
  const visit = useRef<AbortController | null>(null);
  const action = useMutation({
    mutationFn: ({ taskId, signal }: { taskId: number; signal: AbortSignal }) => dismissTaskCompletions(taskId, signal),
    onSettled: () => { void client.invalidateQueries({ queryKey: ["inbox"] }); },
  });
  const { mutate } = action;
  useEffect(() => {
    const controller = new AbortController();
    visit.current = controller;
    if (taskId !== undefined) mutate({ taskId, signal: controller.signal });
    // A delayed snapshot must never dismiss reports after leaving this task.
    return () => controller.abort();
  }, [taskId, mutate]);
  return {
    ...action,
    retry: () => {
      if (taskId !== undefined && visit.current && !visit.current.signal.aborted) {
        mutate({ taskId, signal: visit.current.signal });
      }
    },
  };
}
