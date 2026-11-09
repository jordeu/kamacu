import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { get, post, put } from "./client";
export interface NotificationRule {
  id: string;
  revision: number;
  text: string;
  enabled: boolean;
}
export interface NotificationRuleSet {
  id: string;
  revision: number;
  status: string;
  rules: NotificationRule[];
}
export interface RuleDecision {
  batch_id: string;
  rule_set_id: string;
  rule_set_revision: number;
  activity_id: string;
  event_ids: number[];
  outcome: string;
  reason: string;
  rule_ids: string[];
  evidence_refs: string[];
  facts?: Record<string, unknown>;
  evaluations?: unknown[];
  semantic?: unknown[];
}
export interface RulePreview {
  id: string;
  rule_set_id: string;
  status: string;
  error: string;
  result: {
    messages?: {
      summary: string;
      context: string;
      question: string;
      why_it_matters: string;
      rule_ids: string[];
    }[];
    decisions?: {
      event_ids: number[];
      outcome: string;
      reason: string;
      rule_ids: string[];
    }[];
  };
}
export function useNotificationRules() {
  return useQuery({
    queryKey: ["notification-rules"],
    queryFn: () => get<NotificationRuleSet[]>("/api/notifications/rules"),
  });
}
export function useSaveNotificationRules() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { base_revision: number; rules: NotificationRule[] }) =>
      put<NotificationRuleSet>("/api/notifications/rules", input),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["notification-rules"] }),
  });
}
export function useStartRulePreview() {
  return useMutation({
    mutationFn: (id: string) =>
      post<RulePreview>("/api/notifications/rules/previews", {
        rule_set_id: id,
      }),
  });
}
export function useRulePreview(id: string) {
  return useQuery({
    queryKey: ["notification-rule-preview", id],
    queryFn: () => get<RulePreview>(`/api/notifications/rules/previews/${id}`),
    enabled: !!id,
    refetchInterval: (q) =>
      ["completed", "failed"].includes(q.state.data?.status ?? "")
        ? false
        : 1000,
  });
}
export interface NotificationConfig {
  profile_id: number | null;
  revision: number;
  context_bytes: number;
  output_tokens: number;
  timeout_seconds: number;
  max_reads: number;
}
export function useNotificationConfig() {
  return useQuery({
    queryKey: ["notification-config"],
    queryFn: () => get<NotificationConfig>("/api/notifications/config"),
  });
}
