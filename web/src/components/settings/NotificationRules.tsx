import { useState } from "react";
import {
  useNotificationRules,
  useSaveNotificationRules,
  useStartRulePreview,
  useRulePreview,
  type NotificationRuleSet,
  type NotificationRule,
} from "@/api/notificationRules";
import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";

export function NotificationRules() {
  const q = useNotificationRules();
  if (q.isPending) return <p>Loading notification rules…</p>;
  if (q.isError)
    return (
      <p role="alert">
        Could not load notification rules.{" "}
        <button onClick={() => void q.refetch()}>Retry</button>
      </p>
    );
  return <RuleEditor initial={q.data[0]} />;
}
function RuleEditor({ initial }: { initial: NotificationRuleSet }) {
  const [edited, setEdited] = useState<NotificationRule[] | null>(null);
  const [base, setBase] = useState(initial.revision);
  const [previewID, setPreviewID] = useState("");
  const save = useSaveNotificationRules(),
    start = useStartRulePreview(),
    preview = useRulePreview(previewID);
  const rules = edited ?? initial.rules;
  const stale = edited !== null && base !== initial.revision;
  const change = (value: NotificationRule[]) => {
    if (edited === null) setBase(initial.revision);
    setEdited(value);
  };
  return (
    <section
      className="space-y-3 rounded border p-3"
      aria-label="Notification rules"
    >
      <h3 className="text-sm font-medium">
        Notification rules · revision {initial.revision}
      </h3>
      <p className="text-xs text-muted-foreground">
        The notification model reads these instructions directly. IDs stay
        stable so each message can explain which rules mattered. Saving makes
        this list effective for new runs.
      </p>
      {initial.status !== "active" && (
        <p className="text-xs">
          This is a preserved unsaved revision. Review and save it to use these
          instructions.
        </p>
      )}
      {rules.map((r, i) => (
        <div
          key={r.id || `new-${i}`}
          className="space-y-2 rounded bg-muted/40 p-3"
        >
          <div className="flex items-center justify-between">
            <label className="text-xs">
              <input
                type="checkbox"
                disabled={save.isPending}
                checked={r.enabled}
                onChange={(e) =>
                  change(
                    rules.map((v, j) =>
                      i === j ? { ...v, enabled: e.target.checked } : v,
                    ),
                  )
                }
              />{" "}
              {r.id || "New rule"}
            </label>
            <Button
              size="sm"
              variant="ghost"
              disabled={save.isPending}
              onClick={() => change(rules.filter((_, j) => i !== j))}
            >
              Remove
            </Button>
          </div>
          <Textarea
            aria-label={`Rule ${r.id || i + 1}`}
            value={r.text}
            maxLength={8000}
            rows={3}
            disabled={save.isPending}
            onChange={(e) =>
              change(
                rules.map((v, j) =>
                  i === j ? { ...v, text: e.target.value } : v,
                ),
              )
            }
          />
        </div>
      ))}
      {stale && (
        <p role="alert">
          Rules changed while you were editing. Your edits are kept. Reload the
          current list before saving.
        </p>
      )}
      <div className="flex flex-wrap gap-2">
        <Button
          size="sm"
          variant="outline"
          disabled={save.isPending || rules.length >= 100}
          onClick={() =>
            change([...rules, { id: "", revision: 0, text: "", enabled: true }])
          }
        >
          Add rule
        </Button>
        <Button
          size="sm"
          disabled={
            save.isPending ||
            stale ||
            (edited === null && initial.status === "active") ||
            rules.some((r) => !r.text.trim())
          }
          onClick={() =>
            save.mutate(
              {
                base_revision: edited === null ? initial.revision : base,
                rules,
              },
              { onSuccess: () => setEdited(null) },
            )
          }
        >
          Save rules
        </Button>
        {edited !== null && (
          <Button
            size="sm"
            variant="ghost"
            disabled={save.isPending}
            onClick={() => {
              setEdited(null);
              setBase(initial.revision);
            }}
          >
            Discard edits and reload
          </Button>
        )}
        <Button
          size="sm"
          variant="outline"
          disabled={edited !== null || save.isPending || start.isPending}
          onClick={() =>
            start.mutate(initial.id, { onSuccess: (r) => setPreviewID(r.id) })
          }
        >
          Preview recent notifications
        </Button>
      </div>
      {(save.error || start.error || preview.error) && (
        <p role="alert">
          {(save.error || start.error || preview.error)?.message}
        </p>
      )}
      {previewID && (
        <div className="space-y-2 text-xs">
          <p>
            Preview {preview.data?.status ?? "loading"}. Uses the selected
            notification model; no messages or tasks are created.
          </p>
          {preview.data?.error && <p role="alert">{preview.data.error}</p>}
          {preview.data?.result.messages?.map((m, i) => (
            <div key={i} className="rounded bg-muted p-2">
              <strong>{m.question}</strong>
              <p>{m.summary}</p>
              <p>
                {m.why_it_matters} · {m.rule_ids.join(", ")}
              </p>
            </div>
          ))}
          {preview.data?.result.decisions?.map((d, i) => (
            <p key={i}>
              {d.outcome}: {d.reason} {d.rule_ids.join(", ")}
            </p>
          ))}
        </div>
      )}
    </section>
  );
}
