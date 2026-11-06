import { useState } from "react";
import { useAddNotificationSource, useEnableNotificationSource, useNotificationSources, useRemoveNotificationSource } from "@/api/notifications";
import type { Settings } from "@/api/settings";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { NotificationRules } from "./NotificationRules";


export function NotificationsSection({ settings }: { settings: Settings }) {
  const sources = useNotificationSources();
  const add = useAddNotificationSource();
  const enable = useEnableNotificationSource();
  const remove = useRemoveNotificationSource();
  const [name, setName] = useState("GitHub");
  const githubEnabled = settings.github_integration?.value === "on";
  return <section className="flex flex-col gap-3" aria-labelledby="notifications-heading">
    <h2 id="notifications-heading" className="text-xs font-medium tracking-wide text-muted-foreground uppercase">Notification sources</h2>
    <p className="text-xs text-muted-foreground">New activity in registered repositories is checked using your GitHub CLI login. Review the raw list in Notifications and choose Process pending when ready. Related activity is classified and grouped into Inbox actions. GitHub read state stays unchanged.</p>
    {!githubEnabled && <p className="text-xs text-muted-foreground">GitHub collection and actions are paused while GitHub integration is off.</p>}
    {sources.isLoading && <p className="text-sm">Loading sources…</p>}
    {sources.isError && <p role="alert" className="text-sm text-destructive">Could not load notification sources. <button type="button" className="underline" onClick={() => void sources.refetch()}>Retry</button></p>}
    {sources.data?.map(source => <div key={source.id} className="space-y-2 rounded border border-border p-3">
      <div className="flex items-center justify-between gap-3">
        <p className="text-sm font-medium">{source.name}{source.account && <span className="font-normal text-muted-foreground"> · {source.account}</span>}</p>
        <Button size="sm" variant="outline" disabled={enable.isPending} onClick={() => enable.mutate({ id: source.id, enabled: !source.enabled })}>{source.enabled ? "Pause" : "Resume"}</Button>
      </div>
      <p className="text-xs text-muted-foreground">{!source.enabled ? "Paused" : !githubEnabled ? "GitHub integration off" : source.last_success ? `Last checked ${new Date(source.last_success).toLocaleString()}` : "Waiting for first check"}{source.pending_events > 0 && ` · ${source.pending_events} pending`}</p>
      {(source.last_error || source.processing_error) && <p role="status" className="break-words text-xs text-destructive">{source.last_error || source.processing_error}</p>}
      <Button size="sm" variant="ghost" disabled={remove.isPending} onClick={() => remove.mutate(source.id)}>Disconnect</Button>
    </div>)}
    {sources.data && !sources.data.some(source => source.provider === "github") && <form className="flex items-end gap-2" onSubmit={event => { event.preventDefault(); if (name.trim()) add.mutate(name.trim()); }}>
      <label className="min-w-0 flex-1 text-xs">Source name<Input value={name} maxLength={120} onChange={event => setName(event.target.value)} className="mt-1" /></label>
      <Button type="submit" variant="outline" disabled={add.isPending || !name.trim() || !githubEnabled}>{add.isPending ? "Adding…" : "Add GitHub source"}</Button>
    </form>}
    {(add.isError || enable.isError || remove.isError) && <p role="alert" className="text-sm text-destructive">{add.error?.message || enable.error?.message || remove.error?.message}</p>}
    <NotificationRules />
  </section>;
}
