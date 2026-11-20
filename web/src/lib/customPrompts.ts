export interface CustomPrompt {
  title: string;
  text: string;
}

export interface CustomPromptDraft {
  index: number;
  prompt: CustomPrompt;
  source: string;
}

// An editor's index is only valid for the list it was opened against. A
// settings refresh (including another browser tab) may remove or reorder it.
export function applyCustomPromptDraft(value: string | undefined, draft: CustomPromptDraft): CustomPrompt[] {
  if ((value ?? "[]") !== draft.source) {
    throw new Error("Custom prompts changed while you were editing. Copy your changes, then reopen the prompt and try again.");
  }
  const prompts = parseCustomPrompts(value);
  const prompt = { title: draft.prompt.title.trim(), text: draft.prompt.text };
  if (draft.index === -1) prompts.push(prompt);
  else prompts[draft.index] = prompt;
  return prompts;
}

export function parseCustomPrompts(value: string | undefined): CustomPrompt[] {
  try {
    const prompts: unknown = JSON.parse(value ?? "[]");
    if (!Array.isArray(prompts)) return [];
    return prompts.filter((prompt): prompt is CustomPrompt =>
      prompt !== null && typeof prompt === "object" &&
      typeof prompt.title === "string" && prompt.title.trim() !== "" &&
      typeof prompt.text === "string" && prompt.text.trim() !== "",
    );
  } catch {
    return [];
  }
}
