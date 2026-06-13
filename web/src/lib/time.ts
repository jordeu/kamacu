/** Age of an ISO timestamp relative to `now` (ms): "12s", "7m", "1h 5m".
 *  Lifted from QuotaIndicator so the quota footer and PR cards share one
 *  implementation (no date library — UI-SPEC: zero new npm deps). */
export function formatAgo(iso: string | null, now: number): string {
  if (iso === null) return "0s";
  const secs = Math.max(0, Math.floor((now - Date.parse(iso)) / 1000));
  if (secs < 60) return `${secs}s`;
  const mins = Math.floor(secs / 60);
  if (mins < 60) return `${mins}m`;
  return `${Math.floor(mins / 60)}h ${mins % 60}m`;
}
