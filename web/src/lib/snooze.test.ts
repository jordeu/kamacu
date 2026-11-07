import { describe, expect, it } from "vitest";
import { isFutureSnoozeTime, localDateTimeValue, parseCustomSnoozeTime, snoozeTime } from "./snooze";

describe("snooze presets", () => {
  it("uses elapsed time for relative options", () => {
    const now = new Date("2026-11-07T23:45:00Z");
    for (const [preset, minutes] of [["In 30 minutes", 30], ["In 1 hour", 60], ["In 3 hours", 180]] as const) {
      expect(snoozeTime(preset, now).getTime() - now.getTime()).toBe(minutes * 60_000);
    }
  });

  it("uses 9 AM tomorrow in local time, including month and year boundaries", () => {
    const now = new Date(2026, 11, 31, 23, 45);
    expect(localDateTimeValue(snoozeTime("Tomorrow at 9:00 AM", now))).toBe("2027-01-01T09:00");
    expect(now.getHours()).toBe(23);
  });

  it("selects the next future Monday at 9 AM", () => {
    for (const [day, hour, minute, expected] of [
      [8, 14, 0, "2026-11-09T09:00"], // Sunday
      [9, 8, 59, "2026-11-09T09:00"], // Today, before 9
      [9, 9, 0, "2026-11-16T09:00"], // Exactly 9
      [9, 10, 0, "2026-11-16T09:00"],
      [10, 8, 0, "2026-11-16T09:00"],
    ] as const) {
      expect(localDateTimeValue(snoozeTime("Monday at 9:00 AM", new Date(2026, 10, day, hour, minute)))).toBe(expected);
    }
  });

  it("keeps calendar time at 9 AM across daylight saving changes", () => {
    // Run under Europe/Madrid too, so these span the spring and autumn transitions.
    for (const now of [new Date(2026, 2, 28, 12), new Date(2026, 9, 24, 12)]) {
      const tomorrow = snoozeTime("Tomorrow at 9:00 AM", now);
      expect(tomorrow.getHours()).toBe(9);
      expect(tomorrow.getDate()).toBe(now.getDate() + 1);
    }
  });
});


it("rejects invalid, current, and past custom times", () => {
  const now = new Date("2026-11-07T09:00:00Z");
  expect(isFutureSnoozeTime(new Date("invalid"), now)).toBe(false);
  expect(isFutureSnoozeTime(now, now)).toBe(false);
  expect(isFutureSnoozeTime(new Date(now.getTime() - 1), now)).toBe(false);
  expect(isFutureSnoozeTime(new Date(now.getTime() + 1), now)).toBe(true);
});


it("rejects malformed or normalized custom dates", () => {
  for (const value of ["", "invalid", "2027-02-30T09:00", "2027-01-01", "2027-01-01T25:00"]) {
    expect(parseCustomSnoozeTime(value)).toBeNull();
  }
  expect(localDateTimeValue(parseCustomSnoozeTime("2027-01-01T09:00")!)).toBe("2027-01-01T09:00");
});

it("rejects a nonexistent custom time during a daylight saving transition", () => {
  // Europe/Madrid skips 02:00–02:59 on March 29, 2026.
  const value = "2026-03-29T02:30";
  const normalized = new Date(value);
  if (normalized.getHours() !== 2) {
    expect(parseCustomSnoozeTime(value)).toBeNull();
  }
});
