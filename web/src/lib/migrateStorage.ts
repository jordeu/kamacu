/**
 * One-shot, idempotent localStorage prefix migration (MIGRATE-04 / D-14).
 *
 * Phase 20 renamed the app kangent → kamacu but deliberately left the browser
 * localStorage key literals on the `kangent` prefix so this phase could move them
 * atomically. This copies every `kangent`-prefixed value to the matching
 * `kamacu`-prefixed key on first boot, so sidebar state, the sessions-bar collapse
 * preference, and per-project review-collapse state carry over the rebrand.
 *
 * The three live keys do NOT share a single separator — the sidebar key uses a
 * dot, the sessions-bar-collapsed key uses a colon, and the per-project
 * review-collapsed key uses a colon plus a dynamic projectId suffix — so a prefix
 * scan over ALL keys keyed on the separator-agnostic bare word `"kangent"` is
 * required (a hard-coded key list would miss the dynamic per-project keys —
 * RESEARCH Pitfall 3).
 *
 * Old keys are left in place (harmless residue): the copy only writes a `kamacu.*`
 * key when it is absent, and a one-shot guard flag makes subsequent boots a no-op.
 * Must run ONCE before the first React render (see main.tsx) so components read the
 * new keys on mount.
 */
export function migrateStorage(): void {
  // Idempotency guard: once migrated, this is a cheap no-op on every later boot.
  if (localStorage.getItem("kamacu.storage-migrated") === "1") return;

  // Index scan over ALL keys (not a fixed list) so the dynamic per-project
  // review-collapsed keys are covered too.
  for (let i = 0; i < localStorage.length; i++) {
    const key = localStorage.key(i);
    // Bare-word `kangent` prefix matches the dot, colon, AND dynamic key shapes.
    if (!key || !key.startsWith("kangent")) continue;
    const newKey = "kamacu" + key.slice("kangent".length);
    // Never clobber an existing kamacu.* value (keeps the copy re-runnable if the
    // guard flag is ever cleared).
    if (localStorage.getItem(newKey) === null) {
      localStorage.setItem(newKey, localStorage.getItem(key)!);
    }
  }

  localStorage.setItem("kamacu.storage-migrated", "1");
}
