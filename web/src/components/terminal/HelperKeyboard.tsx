import { Button } from "@/components/ui/button";
import { cursorKey, helperKeyRows } from "./helperKeys";

export function HelperKeyboard({ id, disabled, applicationCursorKeys, onInput }: {
  id: string;
  disabled: boolean;
  applicationCursorKeys: () => boolean;
  onInput: (data: string) => void;
}) {
  return (
    <div id={id} role="group" aria-label="Terminal helper keyboard" className="max-h-[40%] shrink-0 overflow-y-auto overscroll-contain border-t border-border bg-background p-1">
      <p className="px-1 text-xs text-muted-foreground md:hidden">Swipe rows for more keys</p>
      {helperKeyRows.map((row, index) => (
        <div key={index} className="flex gap-1 overflow-x-auto py-1" aria-label={index === 1 ? "Shortcuts and navigation (scroll for more keys)" : undefined}>
          {row.map((key) => (
            <Button
              key={key.name}
              type="button"
              variant="secondary"
              disabled={disabled}
              aria-label={key.name}
              title={key.name}
              className="h-11 min-w-11 px-2 font-mono text-xs touch-manipulation"
              // Preserve xterm focus and the native keyboard on touch/mouse.
              onPointerDown={(event) => event.preventDefault()}
              onClick={() => onInput("cursor" in key ? cursorKey(key.cursor, applicationCursorKeys()) : key.data)}
            >
              {key.label}
            </Button>
          ))}
        </div>
      ))}
    </div>
  );
}
