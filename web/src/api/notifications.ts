import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { del, get, patch, post } from "./client";

export interface NotificationSource {
  id: string;
  provider: string;
  name: string;
  enabled: boolean;
  account: string;
  last_success: number | null;
  last_error: string;
  processing_error: string;
  pending_events: number;
}
export function useNotificationSources() {
  return useQuery({ queryKey: ["notification-sources"], queryFn: () => get<NotificationSource[]>("/api/notifications/sources"), refetchInterval: 5000 });
}
export function useAddNotificationSource() {
  const qc = useQueryClient();
  return useMutation({ mutationFn: (name: string) => post<NotificationSource>("/api/notifications/sources", { provider: "github", name, config: {} }), onSuccess: () => qc.invalidateQueries({ queryKey: ["notification-sources"] }) });
}
export function useEnableNotificationSource() {
  const qc = useQueryClient();
  return useMutation({ mutationFn: ({ id, enabled }: { id: string; enabled: boolean }) => patch(`/api/notifications/sources/${id}`, { enabled }), onSuccess: () => qc.invalidateQueries({ queryKey: ["notification-sources"] }) });
}

export function useRemoveNotificationSource() {
 const qc = useQueryClient();
 return useMutation({ mutationFn: (id: string) => del(`/api/notifications/sources/${id}`), onSuccess: () => qc.invalidateQueries({ queryKey: ["notification-sources"] }) });
}

export interface RawNotification {
 processing?: boolean;
 id: number;
 source_id: string;
 source_name: string;
 status: "pending" | "archived";
 revision: number;
 created_at: number;
 event: { external_id: string; version: string; repository: string; url: string; title: string; reason: string; subject_type: string; context: string };
}
export interface NotificationProcessing {
 id: string;
 status: "queued" | "running" | "completed" | "failed";
 created_at: number;
 updated_at: number;
 error: string;
 action_count: number;
 notification_count: number;
}
export interface NotificationStatus { pending_count: number; processing: NotificationProcessing | null }
export interface RawNotificationPage { items: RawNotification[]; pending_count: number; has_more: boolean }
export function useRawNotifications(status: "pending" | "archived", offset = 0) {
 return useQuery({ queryKey: ["raw-notifications", status, offset], queryFn: () => get<RawNotificationPage>(`/api/notifications?status=${status}&offset=${offset}`), refetchInterval: 3000 });
}
export function useNotificationStatus() {
 return useQuery({ queryKey: ["notification-status"], queryFn: () => get<NotificationStatus>("/api/notifications/status"), refetchInterval: 2000, refetchIntervalInBackground: true });
}
export function useProcessNotifications() {
 const qc = useQueryClient();
 return useMutation({ mutationFn: () => post<NotificationProcessing>("/api/notifications/process"), onSuccess: () => qc.invalidateQueries({ queryKey: ["notification-status"] }) });
}
export function useArchiveNotification() {
 const qc = useQueryClient();
 return useMutation({ mutationFn: ({ id, status }: { id: number; status: "pending" | "archived" }) => patch(`/api/notifications/${id}`, { status }), onSuccess: () => Promise.all([qc.invalidateQueries({ queryKey: ["raw-notifications"] }), qc.invalidateQueries({ queryKey: ["notification-status"] }), qc.invalidateQueries({ queryKey: ["notification-sources"] })]) });
}
