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
