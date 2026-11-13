import type { PRStatusSnapshot } from "@/api/prStatus";

export type StatusTone = "green" | "red" | "amber" | "neutral";
export interface PrimaryPRStatus {
  icon: "pass" | "fail" | "waiting" | "unknown" | "none";
  tone: StatusTone;
  label: string;
}

export function primaryPRStatus(snapshot?: PRStatusSnapshot): PrimaryPRStatus {
  const old = snapshot?.stale ? " · last known" : "";
  if (snapshot?.ci.state === "fail") return { icon: "fail", tone: "red", label: `CI failing${old}` };
  if (snapshot?.humans.state === "changes") return { icon: "fail", tone: "red", label: `Human changes requested${old}` };
  if (!snapshot || snapshot.stale || snapshot.ci.state === "unknown" || snapshot.humans.state === "unknown") {
    return { icon: "unknown", tone: "neutral", label: snapshot?.stale ? "PR status is stale" : "PR status unavailable" };
  }
  if (snapshot.ci.state === "pending" || snapshot.humans.state === "waiting") {
    return { icon: "waiting", tone: "amber", label: snapshot.ci.state === "pending"
      ? (snapshot.humans.state === "waiting" ? "CI and human review pending" : "CI pending") : "Waiting for human review" };
  }
  if (snapshot.ci.state === "pass" && snapshot.humans.state === "approved") {
    return { icon: "pass", tone: "green", label: "Human approved · CI passing" };
  }
  return { icon: "none", tone: "neutral", label: snapshot.humans.state === "none" ? "No human approval" : "No CI result" };
}

export const prStatusLabels: Record<string, string> = {
  pass: "Passing", fail: "Failing", pending: "Pending", approved: "Approved",
  changes: "Changes requested", waiting: "Review requested", commented: "Commented",
  complete: "Completed", clean: "Completed · no new findings", comments: "Review comments", running: "Review running",
  unknown: "Unknown", stale: "Older commit", none: "None",
};

export function statusTone(state: string): StatusTone {
  if (["fail", "changes"].includes(state)) return "red";
  if (["pending", "waiting", "running", "comments"].includes(state)) return "amber";
  if (["pass", "approved"].includes(state)) return "green";
  return "neutral";
}
