import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, get, post, put } from "@/api/client";
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
type Usage = {
  calls: number;
  failures: number;
  timeouts: number;
  last_latency_ms: number;
  input_tokens: number;
  output_tokens: number;
  token_reports: number;
};
type Config = {
  usage?: Record<number, Usage>;
  profiles: Profile[];
  default_profile_id: number | null;
  workspaces: { id: number; name: string; profile_id: number | null }[];
};
const blank: Profile = {
  id: 0,
  name: "",
  backend: "claude_cli",
  model: "",
  endpoint: "",
  executable: "",
  credential_env: "",
  revision: 0,
};
const control =
  "h-9 rounded-md border border-input bg-background px-3 text-sm w-full";
export function MonitoringSection({ workspaceId }: { workspaceId?: number }) {
  const client = useQueryClient();
  const config = useQuery({
    queryKey: ["monitor-config"],
    refetchInterval: 15000,
    queryFn: () => get<Config>("/api/monitor"),
  });
  const [draft, setDraft] = useState<Profile>(blank);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [editing, setEditing] = useState(false);
  async function action(fn: () => Promise<unknown>, success: string) {
    setBusy(true);
    setNotice("");
    try {
      await fn();
      await client.invalidateQueries({ queryKey: ["monitor-config"] });
      setNotice(success);
    } catch (e) {
      setNotice(e instanceof Error ? e.message : "Could not update monitoring");
    } finally {
      setBusy(false);
    }
  }
  const data = config.data;
  if (!data)
    return (
      <p className="text-sm text-muted-foreground">
        {config.isError
          ? "Could not load monitoring settings."
          : "Loading monitoring settings…"}
      </p>
    );
  const global = data.profiles.find((p) => p.id === data.default_profile_id);
  function selection(id: number, name: string, value: number | null) {
    const effective = data?.profiles.find(
      (p) => p.id === (value ?? data?.default_profile_id),
    );
    return (
      <div key={id} className="space-y-1">
        <label className="text-sm" htmlFor={`monitor-scope-${id}`}>
          {name}
        </label>
        <select
          id={`monitor-scope-${id}`}
          className={control}
          disabled={busy}
          value={value ?? ""}
          onChange={(e) =>
            void action(
              () =>
                put("/api/monitor/selection", {
                  workspace_id: id,
                  profile_id: e.target.value ? Number(e.target.value) : null,
                }),
              "Monitoring model updated.",
            )
          }
        >
          <option value="" disabled={id === 0}>
            {id === 0
              ? "Choose a default model"
              : `Use global default${global ? ` (${global.name})` : ""}`}
          </option>
          {data?.profiles.map((p) => (
            <option key={p.id} value={p.id}>
              {p.name}
            </option>
          ))}
        </select>
        <p className="text-xs text-muted-foreground">
          {effective
            ? `${effective.backend.replaceAll("_", " ")} · ${effective.model || "CLI default model"}`
            : "Setup required before starting agents."}
        </p>
      </div>
    );
  }
  return (
    <section className="space-y-3" aria-label="Terminal monitoring">
      <h2 className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
        Terminal monitoring
      </h2>
      <p className="text-sm text-muted-foreground">
        Detects agent status and requests for input from terminal output.
        Notifications stay quiet while you view the terminal. Model changes also
        apply to running sessions.
      </p>
      {workspaceId === undefined
        ? selection(
            0,
            "Global default (including Scratchpad)",
            data.default_profile_id,
          )
        : data.workspaces
            .filter((w) => w.id === workspaceId)
            .map((w) => selection(w.id, w.name, w.profile_id))}
      {workspaceId === undefined && (
        <>
          <details>
            <summary className="cursor-pointer text-sm">
              Workspace models
            </summary>
            <div className="mt-3 space-y-3">
              {data.workspaces.map((w) =>
                selection(w.id, w.name, w.profile_id),
              )}
            </div>
          </details>
          <div className="space-y-2">
            {data.profiles.map((p) => (
              <div key={p.id} className="flex items-center gap-2">
                <div className="flex-1 text-sm">
                  {p.name}
                  <ProfileUsage usage={data.usage?.[p.id]} />
                </div>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={busy}
                  onClick={() => {
                    setDraft(p);
                    setEditing(true);
                  }}
                >
                  Edit
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  disabled={busy}
                  onClick={() =>
                    void action(
                      () =>
                        api(`/api/monitor/profiles/${p.id}`, {
                          method: "DELETE",
                        }),
                      "Profile removed.",
                    )
                  }
                >
                  Remove
                </Button>
              </div>
            ))}
          </div>
          <Button
            variant="outline"
            onClick={() => {
              setDraft(blank);
              setEditing(true);
            }}
          >
            Add monitoring model
          </Button>
          {editing && (
            <form
              className="space-y-3 rounded-md border p-3"
              onSubmit={(e) => {
                e.preventDefault();
                void action(async () => {
                  await post("/api/monitor/profiles", draft);
                  setEditing(false);
                }, "Profile saved. Select it above to enable monitoring.");
              }}
            >
              <label className="block text-sm">
                Name
                <Input
                  required
                  value={draft.name}
                  onChange={(e) => setDraft({ ...draft, name: e.target.value })}
                />
              </label>
              <div>
                <label className="block text-sm" htmlFor="monitor-backend">
                  Connection
                </label>
                <select
                  id="monitor-backend"
                  className={control}
                  value={draft.backend}
                  onChange={(e) =>
                    setDraft({ ...draft, backend: e.target.value })
                  }
                >
                  <option value="claude_cli">
                    Installed Claude Code login
                  </option>
                  <option value="codex_cli">Installed Codex login</option>
                  <option value="openai_compatible">
                    OpenAI-compatible API or local endpoint
                  </option>
                  <option value="anthropic_api">Anthropic API</option>
                </select>
              </div>
              <label className="block text-sm">
                Model
                <Input
                  placeholder={
                    draft.backend.endsWith("_cli")
                      ? "Use CLI default"
                      : "Model ID"
                  }
                  value={draft.model}
                  onChange={(e) =>
                    setDraft({ ...draft, model: e.target.value })
                  }
                />
              </label>
              {draft.backend.endsWith("_cli") ? (
                <>
                  <p className="text-xs text-muted-foreground">
                    Uses the CLI's current login on the Kamacu server. Sign in
                    through that CLI. Subscription limits still apply.
                  </p>
                  <label className="block text-sm">
                    Executable (optional)
                    <Input
                      placeholder={
                        draft.backend === "claude_cli" ? "claude" : "codex"
                      }
                      value={draft.executable}
                      onChange={(e) =>
                        setDraft({ ...draft, executable: e.target.value })
                      }
                    />
                  </label>
                </>
              ) : (
                <>
                  <label className="block text-sm">
                    API base URL
                    <Input
                      required
                      placeholder="http://localhost:11434/v1"
                      value={draft.endpoint}
                      onChange={(e) =>
                        setDraft({ ...draft, endpoint: e.target.value })
                      }
                    />
                  </label>
                  <label className="block text-sm">
                    API key environment variable
                    <Input
                      placeholder="OPENAI_API_KEY (leave empty for unauthenticated local endpoints)"
                      value={draft.credential_env}
                      onChange={(e) =>
                        setDraft({ ...draft, credential_env: e.target.value })
                      }
                    />
                  </label>
                  <p className="text-xs text-muted-foreground">
                    Set this variable on the Kamacu server. Terminal text is
                    sent to the selected endpoint.
                  </p>
                </>
              )}
              <div className="flex gap-2">
                <Button disabled={busy}>Save</Button>
                <Button
                  type="button"
                  variant="outline"
                  disabled={busy}
                  onClick={() =>
                    void action(
                      () => post("/api/monitor/test", draft),
                      "Connection test passed.",
                    )
                  }
                >
                  {busy ? "Working…" : "Test connection"}
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  onClick={() => setEditing(false)}
                >
                  Cancel
                </Button>
              </div>
            </form>
          )}
        </>
      )}
      {notice && (
        <p role="status" className="text-sm">
          {notice}
        </p>
      )}
    </section>
  );
}

function ProfileUsage({ usage }: { usage?: Usage }) {
  if (!usage) return null;
  return (
    <p className="text-xs text-muted-foreground">
      Since server start: {usage.calls} calls · {usage.failures} failed ·{" "}
      {usage.timeouts} timed out · last{" "}
      {(usage.last_latency_ms / 1000).toFixed(1)}s
      {usage.token_reports > 0
        ? ` · ${usage.input_tokens} input / ${usage.output_tokens} output tokens reported`
        : " · token usage not reported"}
    </p>
  );
}
