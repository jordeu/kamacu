import { useState, type FormEvent } from "react";
import { ApiError } from "@/api/client";
import { useSaveSetting, type Settings } from "@/api/settings";
import { applyCustomPromptDraft, parseCustomPrompts, type CustomPrompt, type CustomPromptDraft } from "@/lib/customPrompts";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";
import { Label } from "@/components/ui/label";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@/components/ui/dialog";

export function CustomPromptsSection({ settings }: { settings: Settings }) {
  const prompts = parseCustomPrompts(settings.custom_prompts?.value);
  const save = useSaveSetting("custom_prompts");
  const [editor, setEditor] = useState<CustomPromptDraft | null>(null);
  const [error, setError] = useState<string | null>(null);

  async function persist(next: CustomPrompt[], closeEditor = false) {
    setError(null);
    try {
      await save.mutateAsync(JSON.stringify(next));
      if (closeEditor) setEditor(null);
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Couldn't save. Try again.");
    }
  }

  function submit(event: FormEvent) {
    event.preventDefault();
    if (!editor || save.isPending) return;
    try {
      void persist(applyCustomPromptDraft(settings.custom_prompts?.value, editor), true);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Couldn't save. Try again.");
    }
  }

  function edit(index: number, prompt: CustomPrompt) {
    setError(null);
    setEditor({ index, prompt: { ...prompt }, source: settings.custom_prompts?.value ?? "[]" });
  }

  return (
    <section className="flex flex-col gap-3">
      <h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">Custom prompts</h2>
      <p className="text-sm text-muted-foreground">Save reusable prompts, then select their titles from a task's Agent actions menu. Text is inserted for you to review before sending.</p>
      {prompts.length === 0 && <p className="text-sm text-muted-foreground">No custom prompts yet.</p>}
      {prompts.map((prompt, index) => (
        <div key={index} className="flex items-start gap-3 rounded-lg border p-3">
          <div className="min-w-0 flex-1">
            <p className="truncate text-sm font-medium" title={prompt.title}>{prompt.title}</p>
            <p className="mt-1 line-clamp-2 whitespace-pre-wrap break-words text-xs text-muted-foreground">{prompt.text}</p>
          </div>
          <Button variant="outline" size="sm" disabled={save.isPending} onClick={() => edit(index, prompt)} aria-label={`Edit ${prompt.title}`}>Edit</Button>
          <Button variant="ghost" size="sm" disabled={save.isPending} onClick={() => void persist(prompts.filter((_, i) => i !== index))} aria-label={`Delete ${prompt.title}`}>Delete</Button>
        </div>
      ))}
      {error && !editor && <p role="alert" className="text-xs text-destructive">{error}</p>}
      <Button className="self-start" variant="outline" disabled={save.isPending || prompts.length >= 100} onClick={() => edit(-1, { title: "", text: "" })}>Add prompt</Button>
      <Dialog open={editor !== null} onOpenChange={(open) => { if (!open && !save.isPending) setEditor(null); }}>
        <DialogContent className="sm:max-w-[560px]" aria-describedby={undefined}>
          <DialogHeader><DialogTitle>{editor?.index === -1 ? "Add prompt" : "Edit prompt"}</DialogTitle></DialogHeader>
          {editor && (
            <form onSubmit={submit} className="flex flex-col gap-4">
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="custom-prompt-title">Short title</Label>
                <Input id="custom-prompt-title" autoFocus required maxLength={80} value={editor.prompt.title} disabled={save.isPending} onChange={event => setEditor({ ...editor, prompt: { ...editor.prompt, title: event.target.value } })} />
              </div>
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="custom-prompt-text">Prompt text</Label>
                <Textarea id="custom-prompt-text" className="min-h-48 max-h-96" required maxLength={20000} value={editor.prompt.text} disabled={save.isPending} onChange={event => setEditor({ ...editor, prompt: { ...editor.prompt, text: event.target.value } })} />
              </div>
              {error && <p role="alert" className="text-xs text-destructive">{error}</p>}
              <DialogFooter>
                <Button type="button" variant="ghost" disabled={save.isPending} onClick={() => setEditor(null)}>Cancel</Button>
                <Button type="submit" disabled={save.isPending || !editor.prompt.title.trim() || !editor.prompt.text.trim()}>{save.isPending ? "Saving…" : "Save prompt"}</Button>
              </DialogFooter>
            </form>
          )}
        </DialogContent>
      </Dialog>
    </section>
  );
}
