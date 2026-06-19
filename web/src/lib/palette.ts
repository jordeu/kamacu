/** PROJECT_PALETTE mirrors internal/api/icons.go `projectPalette` VERBATIM
 *  (same 9 hexes, same locked order, lowercase). There is NO palette endpoint
 *  (Phase 18 D-02): the Go slice is the single source of truth and this is the
 *  TS mirror — keep the two byte-for-byte in sync. These hues are the swatch-grid
 *  source (Project settings) AND the ONLY legal <ProjectAvatar> background; never
 *  hardcode a palette hex anywhere else. Letter/text color on every swatch is
 *  fixed white for AA contrast — never derived from the hue.
 *  Members, in order: red, orange, amber, green, teal, blue, indigo, violet, pink. */
export const PROJECT_PALETTE = [
  "#dc2626", "#ea580c", "#d97706", "#16a34a", "#0d9488",
  "#2563eb", "#4f46e5", "#7c3aed", "#db2777",
] as const;
