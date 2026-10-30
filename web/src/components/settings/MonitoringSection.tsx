import { useRef, useState } from "react";
import { Plus } from "lucide-react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, get, post, put } from "@/api/client";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

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
const backendNames: Record<string, string> = {
  claude_cli: "Installed Claude Code login",
  codex_cli: "Installed Codex login",
  openai_compatible: "OpenAI-compatible API or local endpoint",
  anthropic_api: "Anthropic API",
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
  const dialogOpener = useRef<HTMLButtonElement | null>(null);
  function openEditor(profile: Profile, opener: HTMLButtonElement) {
    dialogOpener.current = opener;
    setDraft({ ...profile });
    setNotice("");
    setEditing(true);
  }
  async function action(
    fn: () => Promise<unknown>,
    success: string,
    onSuccess?: () => void,
  ) {
    setBusy(true);
    setNotice("");
    try {
      await fn();
      await client.invalidateQueries({ queryKey: ["monitor-config"] });
      setNotice(success);
      onSuccess?.();
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
            ? `${backendNames[effective.backend] ?? effective.backend} · ${effective.model || "CLI default model"}`
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
          <div className="overflow-hidden rounded-lg border">
            <div className="flex flex-wrap items-center justify-between gap-3 border-b bg-muted/30 p-4">
              <div>
                <h3 className="text-sm font-medium">
                  Monitoring models ({data.profiles.length})
                </h3>
                <p className="mt-1 text-xs text-muted-foreground">
                  Add models here, then choose which one each workspace uses.
                </p>
              </div>
              <Button
                size="sm"
                disabled={busy}
                onClick={(e) => openEditor(blank, e.currentTarget)}
              >
                <Plus className="size-4" />
                Add monitoring model
              </Button>
            </div>
            {data.profiles.length === 0 ? (
              <div className="p-6 text-center">
                <p className="text-sm font-medium">No monitoring models yet</p>
                <p className="mt-1 text-sm text-muted-foreground">
                  Add your first model using an installed agent login, an API,
                  or a local endpoint.
                </p>
              </div>
            ) : (
              <ul aria-label="Monitoring models" className="divide-y">
                {data.profiles.map((p) => (
                  <li
                    key={p.id}
                    className="flex flex-wrap items-center gap-3 p-4"
                  >
                    <div className="min-w-0 flex-1 basis-48 space-y-1">
                      <div className="flex flex-wrap items-center gap-2">
                        <span className="break-words text-sm font-medium">
                          {p.name}
                        </span>
                        {p.id === data.default_profile_id && (
                          <span className="rounded bg-muted px-2 py-0.5 text-xs">
                            Global default
                          </span>
                        )}
                      </div>
                      <p className="break-words text-xs text-muted-foreground">
                        {backendNames[p.backend] ?? p.backend} ·{" "}
                        {p.model || "CLI default model"}
                      </p>
                      <ProfileUsage usage={data.usage?.[p.id]} />
                    </div>
                    <div className="flex shrink-0 gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={busy}
                        aria-label={`Edit ${p.name}`}
                        onClick={(e) => openEditor(p, e.currentTarget)}
                      >
                        Edit
                      </Button>
                      <Button
                        variant="ghost"
                        size="sm"
                        disabled={busy}
                        aria-label={`Remove ${p.name}`}
                        onClick={() =>
                          void action(
                            () =>
                              api(`/api/monitor/profiles/${p.id}`, {
                                method: "DELETE",
                              }),
                            "Model removed.",
                          )
                        }
                      >
                        Remove
                      </Button>
                    </div>
                  </li>
                ))}
              </ul>
            )}
          </div>
          <Dialog
            open={editing}
            onOpenChange={(open) => {
              if (!busy) {
                setEditing(open);
                setNotice("");
              }
            }}
          >
            <DialogContent
              className="max-h-[85dvh] overflow-y-auto sm:max-w-lg"
              showCloseButton={!busy}
              onCloseAutoFocus={(e) => {
                e.preventDefault();
                dialogOpener.current?.focus();
              }}
            >
              <DialogHeader>
                <DialogTitle>
                  {draft.id ? "Edit monitoring model" : "Add monitoring model"}
                </DialogTitle>
                <DialogDescription>
                  Connect a model to detect agent status and requests for input
                  from terminal output.
                </DialogDescription>
              </DialogHeader>
              <form
                className="space-y-4"
                onSubmit={(e) => {
                  e.preventDefault();
                  void action(
                    () => post("/api/monitor/profiles", draft),
                    draft.id
                      ? "Model updated."
                      : "Model added. Choose it as the global default or for a workspace.",
                    () => setEditing(false),
                  );
                }}
              >
                <fieldset disabled={busy} className="space-y-4">
                  <label className="block text-sm">
                    Name
                    <Input
                      required
                      value={draft.name}
                      onChange={(e) =>
                        setDraft({ ...draft, name: e.target.value })
                      }
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
                        Uses the CLI's current login on the Kamacu server. Sign
                        in through that CLI. Subscription limits still apply.
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
                            setDraft({
                              ...draft,
                              credential_env: e.target.value,
                            })
                          }
                        />
                      </label>
                      <p className="text-xs text-muted-foreground">
                        Set this variable on the Kamacu server. Terminal text is
                        sent to the selected endpoint.
                      </p>
                    </>
                  )}
                </fieldset>
                {notice && (
                  <p role="status" className="text-sm">
                    {notice}
                  </p>
                )}
                <div className="flex flex-wrap gap-2 border-t pt-4">
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
                    disabled={busy}
                    onClick={() => {
                      setEditing(false);
                      setNotice("");
                    }}
                  >
                    Cancel
                  </Button>
                </div>
              </form>
            </DialogContent>
          </Dialog>
        </>
      )}
      {notice && !editing && (
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
