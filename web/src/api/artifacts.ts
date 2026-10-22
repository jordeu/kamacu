import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { del, get, patch } from "./client";

export interface Artifact {
  id: string;
  task_id: number;
  title: string;
  revision: string;
  created_at: string;
  updated_at: string;
}

export function useArtifacts(taskId: number) {
  return useQuery({
    queryKey: ["artifacts", taskId],
    queryFn: () => get<Artifact[]>(`/api/tasks/${taskId}/artifacts`),
    enabled: Number.isFinite(taskId),
    refetchInterval: 3000,
  });
}

export function useRenameArtifact(taskId: number) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: ({ id, title }: { id: string; title: string }) =>
      patch<Artifact>(`/api/tasks/${taskId}/artifacts/${id}`, { title }),
    onSuccess: (artifact) => {
      client.setQueryData<Artifact[]>(["artifacts", taskId], (old) =>
        old?.map((item) => item.id === artifact.id ? artifact : item),
      );
      client.invalidateQueries({ queryKey: ["artifacts", taskId] });
    },
  });
}

export function useRemoveArtifact(taskId: number) {
  const client = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => del(`/api/tasks/${taskId}/artifacts/${id}`),
    onSuccess: (_, id) => {
      client.setQueryData<Artifact[]>(["artifacts", taskId], (old) =>
        old?.filter((item) => item.id !== id),
      );
      client.invalidateQueries({ queryKey: ["artifacts", taskId] });
    },
  });
}
