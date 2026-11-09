import { expect, it } from "vitest";
import { mergeSpawnedSession, type TermSession } from "./sessions";

it("keeps an event-refreshed session unique when the spawn response arrives late", () => {
  const spawned: TermSession = {
    id: "new", label: "Agent", status: "running", createdAt: "2026-11-09T00:00:00Z",
  };
  const exited: TermSession = { ...spawned, status: "exited", exitCode: 0 };
  const older: TermSession = { ...spawned, id: "old" };

  expect(mergeSpawnedSession([exited, older], spawned)).toEqual([exited, older]);
  expect(mergeSpawnedSession([older], spawned)).toEqual([spawned, older]);
  expect(mergeSpawnedSession(undefined, spawned)).toEqual([spawned]);
});
