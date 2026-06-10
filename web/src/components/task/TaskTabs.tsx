import type { ReactNode } from "react";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";

/**
 * The task view's tab strip is the architectural seam for the whole product
 * (D-06): Phase 1 ships only `{ id: "description", label: "Description" }`,
 * later phases append Agent, Bash, and Diff tabs to the same `TabDef[]` array.
 */
export interface TabDef {
  id: string;
  label: string;
  content: ReactNode;
}

export function TaskTabs({ tabs }: { tabs: TabDef[] }) {
  if (tabs.length === 0) return null;

  return (
    <Tabs defaultValue={tabs[0].id} className="gap-6">
      {/* Visible strip even with a single tab — the seam must be visible. */}
      <TabsList variant="line" className="w-full justify-start border-b border-border">
        {tabs.map((tab) => (
          <TabsTrigger
            key={tab.id}
            value={tab.id}
            className="flex-none px-2 data-active:after:bg-blue-500"
          >
            {tab.label}
          </TabsTrigger>
        ))}
      </TabsList>
      {tabs.map((tab) => (
        <TabsContent key={tab.id} value={tab.id}>
          {tab.content}
        </TabsContent>
      ))}
    </Tabs>
  );
}
