import { describe, expect, it, vi } from "vitest";
import { attachTwoFingerScroll } from "./touchScroll";

function setup() {
  const inside = {};
  const outside = {};
  const element = Object.assign(new EventTarget(), {
    contains: (target: unknown) => target === inside,
  });
  const scroll = vi.fn();
  const detach = attachTwoFingerScroll(element as HTMLElement, scroll, () => 20);
  const touch = (type: string, points: [number, number, ("inside" | "outside")?][]) => {
    const event = new Event(type, { cancelable: true });
    Object.defineProperty(event, "touches", {
      value: points.map(([identifier, clientY, location]) => ({
        identifier, clientY, target: location === "outside" ? outside : inside,
      })),
    });
    element.dispatchEvent(event);
    return event;
  };
  return { element, scroll, detach, touch };
}

describe("two-finger terminal scrolling", () => {
  it("scrolls in both directions using the average finger movement", () => {
    const { touch, scroll } = setup();
    expect(touch("touchstart", [[1, 100], [2, 120]]).defaultPrevented).toBe(true);
    touch("touchmove", [[2, 160], [1, 140]]);
    expect(scroll).toHaveBeenLastCalledWith(-2);
    touch("touchmove", [[1, 80], [2, 100]]);
    expect(scroll).toHaveBeenLastCalledWith(3);
  });

  it("accumulates small movements into rows", () => {
    const { touch, scroll } = setup();
    touch("touchstart", [[1, 100], [2, 100]]);
    touch("touchmove", [[1, 92], [2, 92]]);
    touch("touchmove", [[1, 84], [2, 84]]);
    expect(scroll).not.toHaveBeenCalled();
    touch("touchmove", [[1, 76], [2, 76]]);
    expect(scroll).toHaveBeenCalledExactlyOnceWith(1);
  });

  it("leaves single-finger gestures and taps alone", () => {
    const { touch, scroll } = setup();
    for (const type of ["touchstart", "touchmove", "touchend"]) {
      expect(touch(type, type === "touchend" ? [] : [[1, 100]]).defaultPrevented).toBe(false);
    }
    expect(scroll).not.toHaveBeenCalled();
  });

  it("does not combine a terminal touch with a finger elsewhere on the page", () => {
    const { touch, scroll } = setup();
    expect(touch("touchstart", [[1, 100], [2, 100, "outside"]]).defaultPrevented).toBe(false);
    expect(touch("touchmove", [[1, 140], [2, 140, "outside"]]).defaultPrevented).toBe(false);
    expect(scroll).not.toHaveBeenCalled();
  });

  it("finishes when terminal fingers lift even if an unrelated finger stays down", () => {
    const { touch, scroll } = setup();
    touch("touchstart", [[1, 100], [2, 100], [3, 100, "outside"]]);
    touch("touchmove", [[1, 120], [2, 120], [3, 100, "outside"]]);
    expect(scroll).toHaveBeenCalledExactlyOnceWith(-1);
    touch("touchend", [[3, 100, "outside"]]);
    expect(touch("touchstart", [[4, 100], [3, 100, "outside"]]).defaultPrevented).toBe(false);
  });

  it("keeps the remaining finger captured after a partial cancellation", () => {
    const { touch, scroll } = setup();
    touch("touchstart", [[1, 100], [2, 100]]);
    touch("touchcancel", [[1, 100]]);
    expect(touch("touchmove", [[1, 200]]).defaultPrevented).toBe(true);
    expect(scroll).not.toHaveBeenCalled();
    touch("touchend", []);
    expect(touch("touchstart", [[3, 100]]).defaultPrevented).toBe(false);
  });

  it("captures the gesture until all fingers lift and resets for the next gesture", () => {
    const { element, touch, scroll } = setup();
    const downstream = vi.fn();
    element.addEventListener("touchmove", downstream);
    touch("touchstart", [[1, 100], [2, 100]]);
    touch("touchmove", [[1, 90], [2, 90]]);
    expect(downstream).not.toHaveBeenCalled();
    expect(touch("touchend", [[1, 90]]).defaultPrevented).toBe(true);
    touch("touchmove", [[1, 10]]);
    touch("touchend", []);
    expect(touch("touchstart", [[3, 100]]).defaultPrevented).toBe(false);
    touch("touchstart", [[3, 100], [4, 100]]);
    touch("touchmove", [[3, 90], [4, 90]]);
    expect(scroll).not.toHaveBeenCalled();
  });

  it("rebases when fingers are added, replaced, or canceled", () => {
    const { touch, scroll } = setup();
    touch("touchstart", [[1, 100], [2, 100]]);
    touch("touchstart", [[1, 100], [2, 100], [3, 200]]);
    touch("touchend", [[1, 100], [3, 200]]);
    touch("touchmove", [[1, 120], [3, 220]]);
    expect(scroll).toHaveBeenCalledExactlyOnceWith(-1);
    touch("touchmove", [[4, 400], [3, 220]]);
    expect(scroll).toHaveBeenCalledTimes(1);
    touch("touchcancel", []);
    expect(touch("touchmove", [[1, 100]]).defaultPrevented).toBe(false);
  });

  it("removes listeners when the terminal is disposed", () => {
    const { touch, scroll, detach } = setup();
    touch("touchstart", [[1, 100], [2, 100]]);
    detach();
    expect(touch("touchmove", [[1, 200], [2, 200]]).defaultPrevented).toBe(false);
    expect(scroll).not.toHaveBeenCalled();
  });
});
