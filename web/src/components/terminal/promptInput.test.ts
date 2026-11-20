import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { createPromptInput } from "./promptInput";

beforeEach(() => vi.useFakeTimers());
afterEach(() => vi.useRealTimers());

function setup() {
  const term = { paste: vi.fn(), input: vi.fn(), modes: { bracketedPasteMode: true }, options: { disableStdin: false } };
  const input = createPromptInput(term);
  input.setConnected(true);
  return { term, input };
}

describe("prompt execution", () => {
  it.each(["Explain changes.", "First line.\n\nSecond line: café 🚀\n", "x".repeat(20000)])("pastes the full text before sending exactly one separate Enter (%#)", (text) => {
    const { term, input } = setup();
    input.sendPrompt(text);
    expect(term.paste).toHaveBeenCalledExactlyOnceWith(text);
    expect(term.input).not.toHaveBeenCalled();
    vi.advanceTimersByTime(99);
    expect(term.input).not.toHaveBeenCalled();
    vi.advanceTimersByTime(1);
    expect(term.input).toHaveBeenCalledExactlyOnceWith("\r", true);
    vi.runAllTimers();
    expect(term.input).toHaveBeenCalledTimes(1);
  });

  it("keeps rapid selections from combining two prompts", () => {
    const { term, input } = setup();
    input.sendPrompt("First");
    input.sendPrompt("Second");
    expect(term.paste).toHaveBeenCalledExactlyOnceWith("First");
    vi.runAllTimers();
    input.sendPrompt("Second");
    vi.runAllTimers();
    expect(term.paste.mock.calls).toEqual([["First"], ["Second"]]);
    expect(term.input).toHaveBeenCalledTimes(2);
  });

  it("cancels Enter on disconnect, including a reconnect before the timer fires", () => {
    const { term, input } = setup();
    input.sendPrompt("First");
    input.setConnected(false);
    input.sendPrompt("Lost");
    input.setConnected(true);
    vi.runAllTimers();
    expect(term.input).not.toHaveBeenCalled();
    expect(term.paste).toHaveBeenCalledExactlyOnceWith("First");
    input.sendPrompt("New connection");
    vi.runAllTimers();
    expect(term.input).toHaveBeenCalledExactlyOnceWith("\r", true);
  });

  it("cancels pending submission when the terminal is replaced or unmounted", () => {
    const { term, input } = setup();
    input.sendPrompt("First");
    input.dispose();
    input.sendPrompt("Disposed");
    vi.runAllTimers();
    expect(term.input).not.toHaveBeenCalled();
    expect(term.paste).toHaveBeenCalledExactlyOnceWith("First");
  });

  it("does not send an extra Enter after the user submits the prompt manually", () => {
    const { term, input } = setup();
    input.sendPrompt("First");
    vi.advanceTimersByTime(50);
    input.cancelSubmit();
    term.input("\r", true);
    vi.runAllTimers();
    expect(term.input).toHaveBeenCalledExactlyOnceWith("\r", true);
  });

  it("allows the user to take over editing without submitting their changes", () => {
    const { term, input } = setup();
    input.sendPrompt("First");
    vi.advanceTimersByTime(50);
    input.cancelSubmit();
    term.input("edited", true);
    vi.runAllTimers();
    expect(term.input).toHaveBeenCalledExactlyOnceWith("edited", true);
    input.sendPrompt("Second");
    vi.runAllTimers();
    expect(term.paste.mock.calls).toEqual([["First"], ["Second"]]);
    expect(term.input.mock.calls).toEqual([["edited", true], ["\r", true]]);
  });

  it("ignores empty prompts and clicks before connection", () => {
    const { term, input } = setup();
    input.setConnected(false);
    expect(input.sendPrompt("Not connected")).toBe(false);
    input.setConnected(true);
    input.sendPrompt(" \n\t");
    vi.runAllTimers();
    expect(term.paste).not.toHaveBeenCalled();
    expect(term.input).not.toHaveBeenCalled();
  });

  it("waits for the CLI to enable bracketed paste before accepting prompts", () => {
    const { term, input } = setup();
    term.modes.bracketedPasteMode = false;
    expect(input.sendPrompt("First line\nSecond line")).toBe(false);
    vi.runAllTimers();
    expect(term.paste).not.toHaveBeenCalled();
    expect(term.input).not.toHaveBeenCalled();
    term.modes.bracketedPasteMode = true;
    expect(input.sendPrompt("First line\nSecond line")).toBe(true);
    vi.runAllTimers();
    expect(term.input).toHaveBeenCalledExactlyOnceWith("\r", true);
  });

  it.each(["paste mode", "stdin"])("does not submit after the CLI disables %s", (mode) => {
    const { term, input } = setup();
    input.sendPrompt("First");
    if (mode === "paste mode") term.modes.bracketedPasteMode = false;
    else term.options.disableStdin = true;
    vi.runAllTimers();
    expect(term.input).not.toHaveBeenCalled();
  });
});
