import { useEffect } from "react";
import { inboxFavicon } from "./inboxFavicon";

export function useInboxFavicon(count: number | undefined) {
  useEffect(() => {
    const link = document.querySelector<HTMLLinkElement>('link[rel="icon"]');
    if (!link || count === undefined || count <= 0) return;

    const original = link.getAttribute("href");
    link.setAttribute("href", inboxFavicon(count));
    return () => {
      if (original === null) link.removeAttribute("href");
      else link.setAttribute("href", original);
    };
  }, [count]);
}
