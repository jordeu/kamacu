import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { get, put, post } from "@/api/client";
import {
  useNotificationConfig,
  type NotificationConfig,
} from "@/api/notificationRules";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";

type Profile = {
  id: number;
  name: string;
  backend: string;
  model: string;
  endpoint: string;
  executable: string;
  credential_env: string;
  revision: number;
};
export function NotificationModel() {
  const qc = useQueryClient(),
    config = useNotificationConfig();
  const profiles = useQuery({
    queryKey: ["model-profiles"],
    queryFn: () => get<{ profiles: Profile[] }>("/api/monitor"),
  });
  const usage = useQuery({
    queryKey: ["notification-usage"],
    queryFn: () =>
      get<
        Record<
          number,
          {
            calls: number;
            failures: number;
            input_tokens: number;
            output_tokens: number;
            last_latency_ms: number;
          }
        >
      >("/api/notifications/usage"),
    refetchInterval: 5000,
  });
  const [draft, setDraft] = useState<NotificationConfig | null>(null),
    [editing, setEditing] = useState(false);
  const [model, setModel] = useState<Profile>({
    id: 0,
    name: "",
    backend: "codex_cli",
    model: "",
    endpoint: "",
    executable: "",
    credential_env: "",
    revision: 0,
  });
  const save = useMutation({
    mutationFn: (value: NotificationConfig) =>
      put<NotificationConfig>("/api/notifications/config", value),
    onSuccess: async () => {
      test.reset();
      setDraft(null);
      await qc.invalidateQueries({ queryKey: ["notification-config"] });
      await qc.invalidateQueries({ queryKey: ["notification-status"] });
    },
  });
  const create = useMutation({
    mutationFn: () => post<Profile>("/api/monitor/profiles", model),
    onSuccess: async (p) => {
      test.reset();
      await qc.invalidateQueries({ queryKey: ["model-profiles"] });
      await qc.invalidateQueries({ queryKey: ["monitor-config"] });
      setEditing(false);
      if (config.data) save.mutate({ ...config.data, profile_id: p.id });
    },
  });
  const test = useMutation({
    mutationFn: () => post<{ ok: boolean }>("/api/notifications/config/test"),
  });
  if (!config.data || !profiles.data)
    return (
      <p>
        {config.error || profiles.error
          ? "Could not load notification model settings."
          : "Loading notification model settings…"}
      </p>
    );
  const c = draft ?? config.data,
    selected = profiles.data.profiles.find((p) => p.id === c.profile_id);
  return (
    <section
      className="space-y-3 rounded border p-3"
      aria-label="Notification processing model"
    >
      <h3 className="text-sm font-medium">Notification processing model</h3>
      <p className="text-xs text-muted-foreground">
        Used for notification decisions, free-text Inbox replies and rule
        suggestions. Saved option actions run without extra model calls. This
        selection is independent of terminal monitoring.
      </p>
      <label className="block text-sm">
        Model
        <select
          className="ml-2 max-w-full rounded border bg-background p-2"
          value={c.profile_id ?? ""}
          disabled={save.isPending || draft !== null}
          onChange={(e) =>
            save.mutate({
              ...config.data!,
              profile_id: e.target.value ? Number(e.target.value) : null,
            })
          }
        >
          <option value="">Choose a notification model</option>
          {profiles.data.profiles.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name} · {p.model || p.backend}
            </option>
          ))}
        </select>
      </label>
      {!c.profile_id && (
        <p role="status" className="text-xs">
          Choose a model before processing notifications. Source collection
          continues.
        </p>
      )}
      {selected && (
        <p className="text-xs">
          {selected.backend} · {selected.model || "Installed CLI default"}
        </p>
      )}
      <div className="flex gap-2">
        <Button
          size="sm"
          variant="outline"
          disabled={draft !== null || create.isPending}
          onClick={() => {
            setModel({
              id: 0,
              name: "",
              backend: "codex_cli",
              model: "",
              endpoint: "",
              executable: "",
              credential_env: "",
              revision: 0,
            });
            setEditing(true);
          }}
        >
          Add notification model
        </Button>
        {selected && (
          <Button
            size="sm"
            variant="outline"
            disabled={draft !== null || create.isPending}
            onClick={() => {
              setModel({ ...selected });
              setEditing(true);
            }}
          >
            Edit selected model
          </Button>
        )}
        <Button
          size="sm"
          variant="outline"
          disabled={
            !config.data.profile_id ||
            save.isPending ||
            test.isPending ||
            draft !== null
          }
          onClick={() => test.mutate()}
        >
          {test.isPending ? "Testing…" : "Test notification model"}
        </Button>
      </div>
      {editing && (
        <div className="space-y-2 rounded border p-3">
          {model.id > 0 && (
            <p className="text-xs">
              This is a reusable profile. Edits affect any other selection using
              the same profile. Add a dedicated notification model to keep its
              configuration separate.
            </p>
          )}
          <label className="block text-xs">
            Name
            <Input
              value={model.name}
              onChange={(e) => setModel({ ...model, name: e.target.value })}
            />
          </label>
          <label className="block text-xs">
            Backend
            <select
              value={model.backend}
              className="ml-2 rounded bg-background p-2"
              onChange={(e) => setModel({ ...model, backend: e.target.value })}
            >
              <option value="codex_cli">Installed Codex login</option>
              <option value="claude_cli">Installed Claude login</option>
              <option value="openai_compatible">OpenAI-compatible API</option>
              <option value="anthropic_api">Anthropic API</option>
            </select>
          </label>
          <label className="block text-xs">
            Model
            <Input
              value={model.model}
              onChange={(e) => setModel({ ...model, model: e.target.value })}
            />
          </label>
          {model.backend.endsWith("api") ||
          model.backend === "openai_compatible" ? (
            <>
              <label className="block text-xs">
                Endpoint
                <Input
                  value={model.endpoint}
                  onChange={(e) =>
                    setModel({ ...model, endpoint: e.target.value })
                  }
                />
              </label>
              <label className="block text-xs">
                Credential environment variable
                <Input
                  value={model.credential_env}
                  onChange={(e) =>
                    setModel({ ...model, credential_env: e.target.value })
                  }
                />
              </label>
            </>
          ) : (
            <label className="block text-xs">
              Executable (optional)
              <Input
                value={model.executable}
                onChange={(e) =>
                  setModel({ ...model, executable: e.target.value })
                }
              />
            </label>
          )}
          <Button
            size="sm"
            disabled={!model.name.trim() || create.isPending || draft !== null}
            onClick={() => create.mutate()}
          >
            Save and select model
          </Button>
        </div>
      )}
      {c.profile_id && usage.data?.[c.profile_id] && (
        <p className="text-xs text-muted-foreground">
          {usage.data[c.profile_id].calls} notification calls ·{" "}
          {usage.data[c.profile_id].failures} failures ·{" "}
          {usage.data[c.profile_id].input_tokens} reported input /{" "}
          {usage.data[c.profile_id].output_tokens} output tokens · last call{" "}
          {usage.data[c.profile_id].last_latency_ms} ms
        </p>
      )}
      {draft !== null && (
        <p className="text-xs">
          Save the edited limits or{" "}
          <button className="underline" onClick={() => setDraft(null)}>
            discard limit edits
          </button>{" "}
          before changing models.
        </p>
      )}
      <details>
        <summary className="cursor-pointer text-xs">Processing limits</summary>
        <p className="text-xs">
          CLI providers use the installed model’s output limits. The context
          budget and run deadline apply to every backend.
        </p>
        <div className="mt-2 grid gap-2 sm:grid-cols-2">
          {(
            [
              ["context_bytes", "Context budget (bytes)", 64000, 8000000],
              ["output_tokens", "API output token limit", 1024, 64000],
              ["timeout_seconds", "Run deadline (seconds)", 30, 900],
              ["max_reads", "Read-only lookups", 0, 40],
            ] as const
          ).map(([key, label, min, max]) => (
            <label key={key} className="text-xs">
              {label}
              <Input
                type="number"
                min={min}
                max={max}
                value={c[key]}
                onChange={(e) =>
                  setDraft({ ...c, [key]: Number(e.target.value) })
                }
              />
            </label>
          ))}
        </div>
        <Button
          size="sm"
          className="mt-2"
          disabled={draft === null || save.isPending}
          onClick={() => save.mutate(c)}
        >
          Save limits
        </Button>
      </details>
      {(save.error || create.error || test.error) && (
        <p role="alert">
          {(save.error || create.error || test.error)?.message}
        </p>
      )}
      {test.data && (
        <p role="status">Notification response contract verified.</p>
      )}
    </section>
  );
}
