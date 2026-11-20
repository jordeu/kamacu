import type { Terminal } from "@xterm/xterm";

export interface PromptInput {
  sendPrompt: (text: string) => boolean;
}

type PromptTerminal = Pick<Terminal, "paste" | "input"> & {
  readonly modes: Pick<Terminal["modes"], "bracketedPasteMode">;
  readonly options: Pick<Terminal["options"], "disableStdin">;
};

// Match Session.WriteMessage: Enter must arrive OUTSIDE the bracketed paste,
// after the CLI has processed it, or Claude/Codex can absorb it into the paste.
const SUBMIT_DELAY_MS = 100;

export function createPromptInput(term: PromptTerminal) {
  let connected = false;
  let disposed = false;
  let submitTimer: ReturnType<typeof setTimeout> | null = null;

  function cancel() {
    if (submitTimer !== null) clearTimeout(submitTimer);
    submitTimer = null;
  }

  function ready() {
    return connected && !disposed && !term.options.disableStdin && term.modes.bracketedPasteMode;
  }

  return {
    // A key, manual paste, or mobile input takes over from automatic submit.
    cancelSubmit: cancel,
    sendPrompt(text: string) {
      // Ignore repeated clicks until this paste has been submitted, so two
      // prompt bodies cannot become a single accidental instruction.
      // A live socket can precede CLI startup or terminal replay. Sending
      // multiline text before paste mode is enabled would execute raw lines.
      if (!ready() || submitTimer !== null || !text.trim()) return false;
      term.paste(text);
      submitTimer = setTimeout(() => {
        submitTimer = null;
        if (ready()) term.input("\r", true);
      }, SUBMIT_DELAY_MS);
      return true;
    },
    setConnected(value: boolean) {
      connected = value;
      if (!value) cancel();
    },
    dispose() {
      disposed = true;
      cancel();
    },
  };
}
