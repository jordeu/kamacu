import { useEffect, useId, useRef, useState } from "react";
import type { ReactNode } from "react";
import { ApiError } from "@/api/client";
import { useSaveSetting } from "@/api/settings";
import type { SettingEntry } from "@/api/settings";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { cn } from "@/lib/utils";

interface SettingsFieldProps {
  settingKey: string;
  label: string;
  entry: SettingEntry;
  help: ReactNode;
  mono?: boolean;
  control?: "input" | "select";
}

/**
 * One setting with the per-field commit state machine (SET-04):
 * - text inputs commit on blur and Enter; no request when the draft equals
 *   the saved value; Esc reverts the draft and keeps focus
 * - the select commits on change immediately
 * - success shows a 2s muted `Saved` flash; the response value becomes the
 *   new revert target (cache replace in useSaveSetting onSuccess)
 * - `Reset to default` renders only while the saved value differs from the
 *   default; click PUTs the default immediately, no confirmation
 * - a 4xx renders the server's canonical message verbatim inline (replacing
 *   the help line); network/5xx renders the fixed copy; the draft is never
 *   cleared and no other field's state is ever touched
 */
export function SettingsField({
  settingKey,
  label,
  entry,
  help,
  mono = false,
  control = "input",
}: SettingsFieldProps) {
  const id = useId();
  const save = useSaveSetting(settingKey);
  const [draft, setDraft] = useState(entry.value);
  const [error, setError] = useState<string | null>(null);
  const [savedFlash, setSavedFlash] = useState(false);
  const flashTimer = useRef<number | null>(null);

  // After a successful save the cache-replaced entry.value is the new revert
  // target — sync the draft to it. A 400 leaves the cache untouched, so the
  // draft survives in place for correction.
  useEffect(() => {
    setDraft(entry.value);
  }, [entry.value]);

  // Clear any pending Saved-flash timer on unmount.
  useEffect(
    () => () => {
      if (flashTimer.current !== null) window.clearTimeout(flashTimer.current);
    },
    [],
  );

  function commit(value: string) {
    save.mutate(value, {
      onSuccess: () => {
        setError(null);
        setSavedFlash(true);
        if (flashTimer.current !== null) {
          window.clearTimeout(flashTimer.current);
        }
        flashTimer.current = window.setTimeout(
          () => setSavedFlash(false),
          2000,
        );
      },
      onError: (err) => {
        if (err instanceof ApiError && err.status < 500) {
          // Canonical server copy, verbatim.
          setError(err.message);
        } else {
          setError(`Couldn't save. Try again.`);
        }
      },
    });
  }

  function commitDraft() {
    // Unchanged draft: no request, no feedback.
    if (draft === entry.value) return;
    // Same value already in flight (Enter then blur): don't double-send.
    if (save.isPending && save.variables === draft) return;
    commit(draft);
  }

  function handleKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === "Enter") {
      commitDraft();
    } else if (e.key === "Escape") {
      // Revert to the last saved value, keep focus (field-level only).
      e.preventDefault();
      setDraft(entry.value);
      setError(null);
    }
  }

  return (
    <div className="flex flex-col gap-1">
      <div className="flex items-center gap-2">
        <Label htmlFor={id} className="text-sm font-medium">
          {label}
        </Label>
        {savedFlash && (
          <span className="text-xs text-muted-foreground">{`Saved`}</span>
        )}
        {entry.value !== entry.default && (
          <button
            type="button"
            className="ml-auto text-xs text-muted-foreground hover:text-foreground disabled:pointer-events-none disabled:opacity-50"
            disabled={save.isPending}
            onClick={() => commit(entry.default)}
          >
            {`Reset to default`}
          </button>
        )}
      </div>
      {control === "select" ? (
        <Select
          value={draft}
          onValueChange={(value) => {
            setDraft(value);
            setError(null);
            commit(value);
          }}
        >
          {/* UI-SPEC: shell select is a compact fixed 240px. */}
          <SelectTrigger id={id} className="w-[240px]">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {entry.options?.map((option) => (
              <SelectItem key={option} value={option}>
                {option}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      ) : (
        <Input
          id={id}
          value={draft}
          onChange={(e) => {
            setDraft(e.target.value);
            setError(null);
          }}
          onBlur={commitDraft}
          onKeyDown={handleKeyDown}
          className={cn(mono && "font-mono")}
        />
      )}
      {error !== null ? (
        <p className="text-xs text-destructive">{error}</p>
      ) : (
        <p className="text-xs text-muted-foreground">{help}</p>
      )}
    </div>
  );
}
