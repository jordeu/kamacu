import { useState } from "react";
import { AlarmClock } from "lucide-react";
import type { Task } from "@/api/types";
import { useTaskSnooze } from "@/api/snooze";
import {
  SNOOZE_PRESETS,
  isFutureSnoozeTime,
  localDateTimeValue,
  parseCustomSnoozeTime,
  snoozeTime,
} from "@/lib/snooze";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuItem,
} from "@/components/ui/dropdown-menu";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from "@/components/ui/dialog";

export function SnoozeTask({ task, compact = false }: { task: Task; compact?: boolean }) {
  const mutation = useTaskSnooze(task);
  const [customOpen, setCustomOpen] = useState(false);
  const [customTime, setCustomTime] = useState("");
  const [validationError, setValidationError] = useState("");
  const snoozed = !!task.snoozed_until;
  if (
    task.source !== "manual" ||
    (!snoozed && task.status !== "in_progress" && task.status !== "in_review")
  ) return null;

  function handleSnooze(until: Date | null) {
    setValidationError("");
    if (until && !isFutureSnoozeTime(until)) {
      setValidationError("Choose a future date and time.");
      return;
    }
    mutation.mutate(until, { onSuccess: () => setCustomOpen(false) });
  }

  return (
    <div
      className={snoozed ? "mt-1 flex flex-wrap items-center gap-1 text-xs" : "shrink-0"}
      onClick={(event) => event.stopPropagation()}
      onPointerDown={(event) => event.stopPropagation()}
      onKeyDown={(event) => event.stopPropagation()}
    >
      {snoozed ? (
        <>
          <span className="text-muted-foreground" title={new Date(task.snoozed_until!).toLocaleString()}>
            Until {new Date(task.snoozed_until!).toLocaleString(undefined, { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" })}
          </span>
          <Button variant="ghost" size="sm" disabled={mutation.isPending} onClick={() => handleSnooze(null)}>
            Reactivate
          </Button>
        </>
      ) : (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size={compact ? "icon-sm" : "sm"} aria-label="Snooze task" title="Snooze task" disabled={mutation.isPending}>
              <AlarmClock className="size-4" />
              {!compact && "Snooze"}
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            {SNOOZE_PRESETS.map((preset) => (
              <DropdownMenuItem key={preset} onSelect={() => handleSnooze(snoozeTime(preset))}>{preset}</DropdownMenuItem>
            ))}
            <DropdownMenuItem onSelect={() => {
              mutation.reset();
              setValidationError("");
              setCustomTime(localDateTimeValue(snoozeTime("In 1 hour")));
              setCustomOpen(true);
            }}>Custom...</DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      )}
      {(mutation.error || validationError) && !customOpen && (
        <p role="alert" className="text-xs text-destructive">{validationError || mutation.error?.message}</p>
      )}
      <Dialog open={customOpen} onOpenChange={setCustomOpen}>
        <DialogContent>
          <form
            className="grid gap-4"
            onSubmit={(event) => {
              event.preventDefault();
              const until = parseCustomSnoozeTime(customTime);
              if (!until) {
                setValidationError("Choose a valid local date and time.");
                return;
              }
              handleSnooze(until);
            }}
          >
            <DialogHeader>
              <DialogTitle>Snooze task</DialogTitle>
              <DialogDescription>Choose when this task returns to {task.status === "in_review" ? "In Review" : "In Progress"}. Times use your local timezone.</DialogDescription>
            </DialogHeader>
            <label className="grid gap-2 text-sm">
              Reactivate at
              <Input type="datetime-local" required value={customTime} min={localDateTimeValue(new Date())} onChange={(event) => setCustomTime(event.target.value)} />
            </label>
            {(validationError || mutation.error) && <p role="alert" className="text-sm text-destructive">{validationError || mutation.error?.message}</p>}
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setCustomOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={mutation.isPending}>{mutation.isPending ? "Snoozing…" : "Snooze"}</Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
