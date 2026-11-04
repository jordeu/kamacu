import { useState } from "react";
import { useSaveSetting, type SettingEntry } from "@/api/settings";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Textarea } from "@/components/ui/textarea";

type Preferences = {
  excluded_repositories?: string[];
  ignore_drafts?: boolean;
  ignore_approved?: boolean;
  ignore_own_activity?: boolean;
  batch_review_task?: string;
};

export function NotificationPreferences({ entry }: { entry: SettingEntry }) {
  return <PreferencesForm key={entry.value} entry={entry} />;
}

function PreferencesForm({ entry }: { entry: SettingEntry }) {
  const initial: Preferences = JSON.parse(entry.value);
  const [preferences, setPreferences] = useState(initial);
  const [repositories, setRepositories] = useState(initial.excluded_repositories?.join("\n") ?? "");
  const save = useSaveSetting("notification_policy");
  const flags = [
    ["ignore_drafts", "Ignore review requests for draft PRs"],
    ["ignore_approved", "Ignore review requests for PRs I have already approved"],
    ["ignore_own_activity", "Ignore notifications caused by my own activity"],
  ] as const;
  return <form className="space-y-3 rounded border border-border p-3" aria-label="Notification preferences" onSubmit={event => {
    event.preventDefault();
    save.mutate(JSON.stringify({ ...preferences, excluded_repositories: repositories.split(/[,\n]/).map(repo => repo.trim()).filter(Boolean) }));
  }}>
    <h3 className="text-sm font-medium">Notification preferences</h3>
    <p className="text-xs text-muted-foreground">Direct mentions take priority over these filters, including draft PRs. When evidence is missing, activity is surfaced for inspection. These preferences apply before the additional instructions below.</p>
    {flags.map(([key, label]) => <label key={key} className="flex items-center gap-2 text-sm"><input type="checkbox" checked={preferences[key] ?? false} onChange={event => setPreferences({ ...preferences, [key]: event.target.checked })} />{label}</label>)}
    <label className="block text-sm">Ignore repositories unless directly mentioned
      <Textarea className="mt-1" rows={3} placeholder="owner/repository, one per line" value={repositories} onChange={event => setRepositories(event.target.value)} />
    </label>
    <label className="block text-sm">Recurrent task for grouped project reviews
      <Input className="mt-1" value={preferences.batch_review_task ?? ""} maxLength={200} onChange={event => setPreferences({ ...preferences, batch_review_task: event.target.value })} />
    </label>
    <p className="text-xs text-muted-foreground">Leave the task blank to keep review requests separate. Choosing a task groups reviews by project; starting work still requires your reply.</p>
    <Button type="submit" size="sm" variant="outline" disabled={save.isPending}>{save.isPending ? "Saving…" : "Save preferences"}</Button>
    {save.isError && <p role="alert" className="text-sm text-destructive">{save.error.message}</p>}
  </form>;
}
