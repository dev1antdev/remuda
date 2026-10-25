// The pure part of a digest schedule: the periods it may cover and the words the list says about it.

import type { DigestSince, Schedule, ScheduleKind } from "./api";

export const SINCE_OPTIONS: { value: DigestSince; label: string }[] = [
  { value: "midnight", label: "Since midnight" },
  { value: "24h", label: "Last 24 h" },
  { value: "7d", label: "Last 7 days" },
  { value: "last", label: "Since the previous digest" },
];

// kindOf reads a schedule's kind, prompt for a record from before the kinds
export const kindOf = (s: Pick<Schedule, "kind">): ScheduleKind => (s.kind === "digest" ? "digest" : "prompt");

// sinceWords says what period a digest covers, as its row reads
export function sinceWords(since: DigestSince | undefined): string {
  switch (since) {
    case "24h":
      return "the last 24 h";
    case "7d":
      return "the last 7 days";
    case "last":
      return "since the previous digest";
    default:
      return "since midnight";
  }
}

// digestSchedule is what a fresh digest looks like before the form is touched
export const digestSchedule = (s: Partial<Schedule>): Schedule => ({
  id: s.id,
  kind: "digest",
  machine: "",
  dir: "",
  label: s.label ?? "",
  cron: s.cron ?? "",
  prompt: "",
  since: s.since ?? "midnight",
  enabled: s.enabled ?? true,
});
