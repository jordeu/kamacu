import type { TermSession } from "@/api/sessions";

export function shouldHandleAgentExit(session: TermSession): boolean {
  return session.kind === "agent" && session.status === "exited" && session.exitSource === "terminal" &&
    (session.exitCode === 0 || session.stopRequested === true);
}

export interface ViewedAgent {
  taskId: number;
  sessionId: string;
}

export function hasViewedAgentExited(
  viewed: ViewedAgent | null,
  taskId: number,
  session: TermSession,
  activelyViewing: boolean,
): boolean {
  return activelyViewing && viewed?.taskId === taskId && viewed.sessionId === session.id &&
    shouldHandleAgentExit(session);
}
