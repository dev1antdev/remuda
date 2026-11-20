// A schedule's kind as the page reads it.

import type { Schedule, ScheduleKind } from "./api";

// kindOf reads a schedule's kind, prompt for a record from before the kinds
export const kindOf = (s: Pick<Schedule, "kind">): ScheduleKind => (s.kind === "agent" || s.kind === "scene" ? s.kind : "prompt");
