import favicon from "../../public/favicon.svg?raw";

// At 16px the count takes priority; the flame tip preserves the brand cue.
export function inboxFavicon(count: number) {
  if (count <= 0) return "/favicon.svg";
  const label = count > 99 ? "99+" : String(count);
  const fontSize = label.length === 1 ? 24 : label.length === 2 ? 22 : 17;
  const svg = favicon.replace('aria-label="Kamacu"', `aria-label="Kamacu: ${label} pending messages"`).replace("</svg>", `
  <rect x="0.75" y="9.25" width="30.5" height="22" rx="6" fill="#4ade80" stroke="#052e16" stroke-width="1.5"/>
  <text x="16" y="27" text-anchor="middle" font-family="Arial,Helvetica,sans-serif" font-size="${fontSize}" font-weight="900" letter-spacing="-1" fill="#052e16">${label}</text>
</svg>`);
  return `data:image/svg+xml,${encodeURIComponent(svg)}`;
}
