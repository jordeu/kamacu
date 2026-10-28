import { useEffect } from "react";
import favicon from "../../public/favicon.svg?raw";

export function useInboxFavicon(count: number | undefined) {
  useEffect(() => {
    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
    if (!link || count === undefined || count <= 0) return;

    const original = link.getAttribute("href");
    const label = count > 99 ? "99+" : String(count);
    const svg = favicon.replace("</svg>", `<rect x="12" y="16" width="20" height="16" rx="8" fill="#f97316" stroke="#1c1917" stroke-width="1"/><text x="22" y="24.5" text-anchor="middle" dominant-baseline="middle" font-family="Arial,sans-serif" font-size="${label.length > 2 ? 9 : 11}" font-weight="700" fill="#1c1917">${label}</text></svg>`);
    link.setAttribute("href", `data:image/svg+xml,${encodeURIComponent(svg)}`);
    return () => {
      if (original === null) link.removeAttribute("href");
      else link.setAttribute("href", original);
    };
  }, [count]);
}
