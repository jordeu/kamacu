import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { get, post } from "./client";

export interface FeedbackQuestion {
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
    onSuccess: () => { void client.invalidateQueries({ queryKey: ["inbox"] }); },
  });
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
    if (result.question.reason === "exit") return "Message archived. Agent session is closing.";
    if (result.question.reason === "seen") return "Marked as seen. Agent session stays open.";
    return "Next prompt sent.";
  }
  return result.question.status === "dismissed" ? "Message dismissed and archived." : "Answer sent. The question has been archived.";
}
