// Keep navigation and screen readers out of the covered app without moving
// the terminal DOM (moving/remounting it would interrupt the live session).
// Newly opened portal menus remain available while the pane is fullscreen.
export function isolateFullscreenPane(element: HTMLElement): () => void {
  const siblings: { element: HTMLElement; inert: boolean }[] = [];
  let branch: HTMLElement | null = element;
  while (branch && branch !== document.body) {
    const parent: HTMLElement | null = branch.parentElement;
    if (!parent) break;
    for (const sibling of parent.children) {
      if (sibling !== branch && sibling instanceof HTMLElement) {
        siblings.push({ element: sibling, inert: sibling.inert });
        sibling.inert = true;
      }
    }
    branch = parent;
  }
  return () => {
    for (const sibling of siblings) sibling.element.inert = sibling.inert;
  };
}

// Android's keyboard can shrink only the visual viewport, leaving 100dvh at
// the layout viewport's height. Follow both its size and pan offsets.
export function followVisualViewport(element: HTMLElement): () => void {
  const viewport = window.visualViewport;
  const update = () => {
    element.style.top = `${viewport?.offsetTop ?? 0}px`;
    element.style.left = `${viewport?.offsetLeft ?? 0}px`;
    element.style.width = `${viewport?.width ?? window.innerWidth}px`;
    element.style.height = `${viewport?.height ?? window.innerHeight}px`;
  };
  update();
  viewport?.addEventListener("resize", update);
  viewport?.addEventListener("scroll", update);
  window.addEventListener("resize", update);
  return () => {
    viewport?.removeEventListener("resize", update);
    viewport?.removeEventListener("scroll", update);
    window.removeEventListener("resize", update);
    for (const property of ["top", "left", "width", "height"]) element.style.removeProperty(property);
  };
}
