import { ChevronDown } from "lucide-react";
import { useRef } from "react";
import { STATUSES, STATUS_LABELS, type Status, type Task } from "@/api/types";
import {
  DropdownMenu,
  DropdownMenuTrigger,
  DropdownMenuContent,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
} from "@/components/ui/dropdown-menu";

export function TaskStatusDropdown({
  task,
  projectName,
  disabled,
  onMove,
}: {
  task: Task;
  projectName: string;
  disabled: boolean;
  onMove: (task: Task, status: Status) => void;
}) {
  const destination = useRef<Status | null>(null);
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          disabled={disabled}
          aria-label={`Change status of ${task.title} · ${projectName}: ${STATUS_LABELS[task.status]}`}
          onClick={(event) => event.stopPropagation()}
          onPointerDown={(event) => event.stopPropagation()}
          onKeyDown={(event) => event.stopPropagation()}
          className="inline-flex min-h-7 shrink-0 items-center gap-1 rounded px-1.5 text-[10px] text-muted-foreground hover:bg-muted hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring disabled:opacity-50"
        >
          {STATUS_LABELS[task.status]}
          <ChevronDown className="size-3" aria-hidden="true" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent
        align="end"
        onCloseAutoFocus={(event) => {
          const status = destination.current;
          destination.current = null;
          if (!status) return;
          // The trigger may move to another column or leave the overview.
          // Keep keyboard focus in the board instead of losing it to the body.
          event.preventDefault();
          const lane = status === "in_review" ? "review" : "progress";
          const cards = document.getElementById(`overview-${lane}-heading`)
            ?.parentElement?.nextElementSibling;
          if (cards instanceof HTMLElement)
            cards.focus({ preventScroll: true });
        }}
        onClick={(event) => event.stopPropagation()}
        onPointerDown={(event) => event.stopPropagation()}
        onKeyDown={(event) => event.stopPropagation()}
      >
        <DropdownMenuRadioGroup
          value={task.status}
          onValueChange={(value) => {
            if (!disabled && value !== task.status) {
              destination.current = value as Status;
              onMove(task, value as Status);
            }
          }}
        >
          {STATUSES.map((status) => (
            <DropdownMenuRadioItem
              key={status}
              value={status}
              className="min-h-9"
            >
              {STATUS_LABELS[status]}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
