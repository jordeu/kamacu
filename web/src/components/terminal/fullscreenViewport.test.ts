import { afterEach, describe, expect, it, vi } from "vitest";
import { followVisualViewport } from "./fullscreenViewport";

afterEach(() => vi.unstubAllGlobals());

function setup(withVisualViewport = true) {
  const viewport = Object.assign(new EventTarget(), { width: 390, height: 800, offsetTop: 0, offsetLeft: 0 });
  const browser = Object.assign(new EventTarget(), {
    visualViewport: withVisualViewport ? viewport : null,
    innerWidth: 390,
    innerHeight: 800,
  });
  vi.stubGlobal("window", browser);
  const style: Record<string, unknown> = {};
  style.removeProperty = (key: string) => { delete style[key]; };
  const detach = followVisualViewport({ style } as unknown as HTMLElement);
  return { viewport, browser, style, detach };
}

describe("fullscreen terminal viewport", () => {
  it("stays above the soft keyboard and follows browser panning", () => {
    const { viewport, style } = setup();
    expect(style).toMatchObject({ width: "390px", height: "800px", top: "0px", left: "0px" });
    viewport.height = 450;
    viewport.dispatchEvent(new Event("resize"));
    expect(style.height).toBe("450px");
    viewport.offsetTop = 60;
    viewport.dispatchEvent(new Event("scroll"));
    expect(style.top).toBe("60px");
    viewport.height = 800;
    viewport.offsetTop = 0;
    viewport.dispatchEvent(new Event("resize"));
    expect(style).toMatchObject({ height: "800px", top: "0px" });
  });

  it("falls back to window resize when VisualViewport is unavailable", () => {
    const { browser, style } = setup(false);
    browser.innerWidth = 800;
    browser.innerHeight = 390;
    browser.dispatchEvent(new Event("resize"));
    expect(style).toMatchObject({ width: "800px", height: "390px" });
  });

  it("restores pane sizing and removes listeners when leaving fullscreen", () => {
    const { viewport, browser, style, detach } = setup();
    detach();
    viewport.dispatchEvent(new Event("resize"));
    viewport.dispatchEvent(new Event("scroll"));
    browser.dispatchEvent(new Event("resize"));
    for (const key of ["width", "height", "top", "left"]) expect(style[key]).toBeUndefined();
  });
});
