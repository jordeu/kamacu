import { useEffect, useId, useRef, type ReactNode } from "react";
import { Link, useSearchParams } from "react-router";
import { AgentIntegrations } from "./AgentIntegrations";
import { AgentsSection } from "./AgentsSection";
import { InboxAgentSection } from "./InboxAgentSection";
import { MonitoringSection } from "./MonitoringSection";
import { NotificationModel } from "./NotificationModel";
import { NotificationRules } from "./NotificationRules";
import { ScratchpadSection } from "./ScratchpadSection";
import { NotificationSchedule } from "@/components/inbox/NotificationSchedule";

const categories = [
  { id: "coding", name: "Coding agents", description: "Choose what launches in project task tabs." },
  { id: "integrations", name: "Kamacu integrations", description: "Connect installed agents to project context and Kamacu tools." },
  { id: "monitoring", name: "Terminal monitoring" },
  { id: "notifications", name: "Notifications", description: "Configure how Kamacu processes and sends notifications." },
  { id: "inbox", name: "Inbox automation", description: "Choose how Kamacu handles eligible new Inbox items." },
  { id: "profiles", name: "Model profiles", description: "Manage reusable models for monitoring, notifications, and Inbox automation." },
  { id: "scratchpad", name: "Scratchpad", description: "Set up agent sessions outside project tasks." },
] as const;

type CategoryId = (typeof categories)[number]["id"];

function categoryParams(params: URLSearchParams, id: CategoryId) {
  const next = new URLSearchParams(params);
  next.set("section", "agents");
  next.set("agent", id);
  return next;
}

export function AgentSettings({ active }: { active: boolean }) {
  const [params, setParams] = useSearchParams();
  const requested = params.get("agent") ?? (params.get("section") === "monitoring" ? "monitoring" : null);
  const current = categories.find(category => category.id === requested)?.id ?? "coding";
  const previous = useRef(active ? current : null);
  const heading = useRef<HTMLHeadingElement>(null);
  const headingPrefix = useId();
  const pickerId = useId();

  useEffect(() => {
    const wasActive = previous.current !== null;
    if (active && wasActive && previous.current !== current) heading.current?.focus();
    previous.current = active ? current : null;
  }, [active, current]);

  const categoryLink = (id: CategoryId) => ({ search: categoryParams(params, id).toString() });
  const manageProfiles = (
    <Link className="inline-flex min-h-9 items-center text-sm font-medium text-primary underline-offset-4 hover:underline" to={categoryLink("profiles")}>
      Manage model profiles
    </Link>
  );
  const content: Record<CategoryId, ReactNode> = {
    coding: <AgentsSection />,
    integrations: <AgentIntegrations />,
    monitoring: <><MonitoringSection view="selection" />{manageProfiles}</>,
    notifications: <><NotificationModel />{manageProfiles}<NotificationRules /><NotificationSchedule /></>,
    inbox: <><InboxAgentSection />{manageProfiles}</>,
    profiles: <MonitoringSection view="profiles" />,
    scratchpad: <ScratchpadSection />,
  };

  return (
    <div className="@container">
      <div className="grid gap-5 @2xl:grid-cols-[11.5rem_minmax(0,1fr)] @2xl:gap-7">
        <nav aria-label="Agent settings categories" className="hidden self-start rounded-lg border bg-muted/30 p-2 @2xl:grid @2xl:gap-1">
          {categories.map(({ id, name }) => (
            <Link
              key={id}
              to={categoryLink(id)}
              aria-current={id === current ? "page" : undefined}
              className="flex min-h-9 items-center rounded-md px-3 py-1.5 text-sm hover:bg-muted focus-visible:outline-2 focus-visible:outline-ring aria-[current=page]:bg-accent aria-[current=page]:font-semibold aria-[current=page]:text-accent-foreground"
            >
              {name}
            </Link>
          ))}
        </nav>
        <div className="space-y-1 @2xl:hidden">
          <label className="block text-sm font-medium" htmlFor={pickerId}>Agent settings category</label>
          <select
            id={pickerId}
            className="min-h-11 w-full rounded-md border border-input bg-background px-3 text-sm"
            value={current}
            onChange={event => {
              const category = categories.find(item => item.id === event.target.value);
              if (category && category.id !== current) setParams(previous => categoryParams(previous, category.id));
            }}
          >
            {categories.map(({ id, name }) => <option key={id} value={id}>{name}</option>)}
          </select>
        </div>
        <div className="min-w-0">
          {/* Preserve draft edits, asynchronous saves and feedback when changing categories. */}
          {categories.map(category => {
            const { id, name } = category;
            return (
              <section key={id} hidden={id !== current} aria-labelledby={`${headingPrefix}-${id}`} className="space-y-4">
                <div className="space-y-1">
                  <h2 id={`${headingPrefix}-${id}`} ref={id === current ? heading : undefined} tabIndex={-1} className="scroll-mt-16 text-xl font-semibold tracking-tight">
                    {name}
                  </h2>
                  {"description" in category && <p className="text-sm text-muted-foreground">{category.description}</p>}
                </div>
                {content[id]}
              </section>
            );
          })}
        </div>
      </div>
    </div>
  );
}
