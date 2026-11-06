import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { get, post } from "./client";

export interface RuleExpression { op: string; fact?: string; value?: unknown; args?: RuleExpression[]; text?: string }
export interface RuleClause { id: string; category: string; when: RuleExpression; effect: string; argument: string; priority: number; on_unknown: string }
export interface NotificationRule { mode?: "compiled" | "llm"; diagnostic?: string; alternatives?: string[]; id: string; revision: number; text: string; enabled: boolean; clauses: RuleClause[]; compiler_version?: string }
export interface NotificationRuleSet { id: string; revision: number; status: "draft" | "compiling" | "ready" | "invalid" | "active" | "retired"; rules: NotificationRule[]; diagnostics: string[] }
export interface RuleDecision { semantic?: { condition: string; result: string; evidence_id?: string; quote?: string }[]; rule_set_id: string; rule_set_revision: number; activity_id: string; outcome: string; evaluations: { rule_id: string; clause_id: string; result: string; effect: string; on_unknown: string; reason?: string }[]; facts: Record<string, { known: boolean; value: unknown; reason?: string }> }
export interface RulePreview { id: string; rule_set_id: string; status: string; error: string; result: { decisions?: RuleDecision[]; cards?: { summary: string; question: string; options: string[] }[] } }
export function useNotificationRules() { return useQuery({ queryKey: ["notification-rules"], queryFn: () => get<NotificationRuleSet[]>("/api/notifications/rules"), refetchInterval: 2000 }); }
export function useRuleMutation(operation: "drafts" | "compile" | "activate" | "preview") {
 const qc = useQueryClient();
 return useMutation({ mutationFn: async (input: { id?: string; base_revision?: number; rules?: NotificationRule[] }): Promise<NotificationRuleSet | RulePreview> => operation === "drafts" ? post<NotificationRuleSet>("/api/notifications/rules/drafts", input) : operation === "preview" ? post<RulePreview>("/api/notifications/rules/previews", { rule_set_id: input.id }) : post<NotificationRuleSet>(`/api/notifications/rules/${input.id}/${operation}`), onSuccess: () => qc.invalidateQueries({ queryKey: ["notification-rules"] }) });
}
export function useRulePreview(id: string) { return useQuery({ queryKey: ["notification-rule-preview", id], queryFn: () => get<RulePreview>(`/api/notifications/rules/previews/${id}`), enabled: !!id, refetchInterval: q => ["completed", "failed"].includes(q.state.data?.status ?? "") ? false : 1000 }); }
export function useRetryHeld() { const qc = useQueryClient(); return useMutation({ mutationFn: () => post("/api/notifications/retry-held"), onSuccess: () => Promise.all([qc.invalidateQueries({ queryKey: ["raw-notifications"] }), qc.invalidateQueries({ queryKey: ["notification-status"] })]) }); }
