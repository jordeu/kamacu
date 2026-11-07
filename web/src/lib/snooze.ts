export const SNOOZE_PRESETS = [
  "In 30 minutes",
  "In 1 hour",
  "In 3 hours",
  "Tomorrow at 9:00 AM",
  "Monday at 9:00 AM",
] as const;

export type SnoozePreset = (typeof SNOOZE_PRESETS)[number];

/** Relative options are elapsed time; calendar options use the browser's local timezone. */
export function snoozeTime(preset: SnoozePreset, now = new Date()): Date {
  const minutes = { "In 30 minutes": 30, "In 1 hour": 60, "In 3 hours": 180 };
  if (preset in minutes) {
    return new Date(now.getTime() + minutes[preset as keyof typeof minutes] * 60_000);
  }
  const result = new Date(now);
  result.setHours(9, 0, 0, 0);
  if (preset === "Tomorrow at 9:00 AM") {
    result.setDate(result.getDate() + 1);
  } else {
    result.setDate(result.getDate() + (1 - result.getDay() + 7) % 7);
    if (result <= now) result.setDate(result.getDate() + 7);
  }
  return result;
}

export function localDateTimeValue(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, "0");
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`;
}

export function isFutureSnoozeTime(date: Date, now = new Date()): boolean {
  return Number.isFinite(date.getTime()) && date.getTime() > now.getTime();
}

/** Reject invalid dates and local clock times skipped by a daylight saving change. */
export function parseCustomSnoozeTime(value: string): Date | null {
  if (!/^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}$/.test(value)) return null;
  const date = new Date(value);
  return Number.isFinite(date.getTime()) && localDateTimeValue(date) === value
    ? date
    : null;
}
