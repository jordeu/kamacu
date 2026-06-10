import { useQuery } from "@tanstack/react-query";
import { get } from "./client";
import type { Project, Task } from "./types";

export function useProjects() {
  return useQuery({
    queryKey: ["projects"],
    queryFn: () => get<Project[]>("/api/projects"),
  });
}

export function useTasks(projectId: number) {
  return useQuery({
    queryKey: ["tasks", projectId],
    queryFn: () => get<Task[]>(`/api/projects/${projectId}/tasks`),
    enabled: !isNaN(projectId),
  });
}

export function useTask(taskId: number) {
  return useQuery({
    queryKey: ["task", taskId],
    queryFn: () => get<Task>(`/api/tasks/${taskId}`),
    enabled: !isNaN(taskId),
  });
}
