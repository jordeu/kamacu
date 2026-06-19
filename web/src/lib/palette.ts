/** PROJECT_PALETTE mirrors internal/api/icons.go `projectPalette` VERBATIM
 *  (same 9 hexes, same locked order, lowercase). There is NO palette endpoint
 *  (Phase 18 D-02): the Go slice is the single source of truth and this is the
 *  TS mirror — keep the two byte-for-byte in sync. These hues are the swatch-grid
 *  source (Project settings) AND the ONLY legal <ProjectAvatar> background; never
 *  hardcode a palette hex anywhere else. The hues are intentionally muted and
 *  desaturated (Phase 19 UAT) so they read as a subtle difference on the dark
 *  sidebar rather than a bright highlight; each still clears WCAG AA (>=4.5:1)
 *  with the fixed white letter color — never derive text color from the hue.
 *  Members, in order: red, orange, amber, green, teal, blue, indigo, violet, pink. */
export const PROJECT_PALETTE = [
  "#9e5757", "#9c6b4b", "#8a7345", "#4e7a54", "#46776f",
  "#5e719c", "#6c6699", "#84689e", "#9c6188",
] as const;
