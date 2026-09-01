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

/** Adaptive largest-1-2-units duration: "1d 3h", "4h 20m", "45m", "30s".
 *  Sibling of formatAgo; input is integer SECONDS (the timeStat wire contract
 *  — int64 seconds truncated server-side, activity_helpers.go:100-102). No date
 *  library (UI-SPEC: zero new npm deps). CONTEXT D-10.
 *
 *  The `n === 0` em-dash guard (CONTEXT D-11) is the CALLER's responsibility
 *  (StatsStrip in Plan 02 checks stat.n before calling this); formatDuration
 *  itself only guards non-finite/negative input. */
export function formatDuration(totalSeconds: number): string {
  if (!Number.isFinite(totalSeconds) || totalSeconds < 0) return "—";
  const s = Math.floor(totalSeconds);
  if (s < 60) return `${s}s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m}m`;
  const h = Math.floor(m / 60);
  const remM = m % 60;
  if (h < 24) return remM > 0 ? `${h}h ${remM}m` : `${h}h`;
  const d = Math.floor(h / 24);
  const remH = h % 24;
  return remH > 0 ? `${d}d ${remH}h` : `${d}d`;
}

/** Absolute "Jul 29, 14:32" — compact month-abbrev + day + comma + 24h HH:MM,
 *  no weekday, no year (D-11). All entries fall within the 7/30-day rolling
 *  Activity window so the year is implicit and the weekday is recoverable from
 *  the chart.
 *
 *  Sibling of formatAgo / formatDuration. Zero deps (D-02 — the Phase-11
 *  zero-new-npm-deps principle is relaxed for recharts ONLY, never for date
 *  libs); uses Date.prototype.toLocaleString with an explicit "en-US" locale
 *  for determinism (MDN warns output varies by host locale — Pitfall 6).
 *
 *  Accepts null AND undefined: ReviewDoneSummary.completedAt is gh-sourced
 *  and could in theory be absent; the null/undefined + NaN guards are
 *  defense-in-depth so a malformed gh closedAt never renders a blank row or
 *  throws (mirrors formatDuration's em-dash sentinel). */
export function formatDateTime(iso: string | null | undefined): string {
  if (!iso) return "—";
  const ms = Date.parse(iso);
  if (Number.isNaN(ms)) return "—";
  return new Date(ms).toLocaleString("en-US", {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  });
}
