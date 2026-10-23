import { describe, expect, it } from "vitest";
import type { Entry } from "./api";
import { groupByDay, unread } from "./group";

const entry = (id: number, at: string): Entry => ({ id, at, kind: "done", machine: "local", title: `e${id}` });

describe("groupByDay", () => {
  it("names today and yesterday, dates the rest, keeping the order given", () => {
    const now = new Date("2026-10-23T15:00:00").getTime();
    const days = groupByDay([entry(5, "2026-10-23T14:00:00"), entry(4, "2026-10-23T09:00:00"), entry(3, "2026-10-22T23:30:00"), entry(2, "2026-10-20T10:00:00"), entry(1, "2026-10-20T08:00:00")], now);
    expect(days.map((d) => [d.label, d.entries.map((e) => e.id)])).toEqual([
      ["Today", [5, 4]],
      ["Yesterday", [3]],
      [expect.stringMatching(/20/) as unknown as string, [2, 1]],
    ]);
    expect(groupByDay([], now)).toEqual([]);
  });
});

describe("unread", () => {
  it("is news when the newest id is past the one seen here", () => {
    expect(unread(12, 10)).toBe(true);
    expect(unread(10, 10)).toBe(false);
    expect(unread(undefined, 0)).toBe(false);
    expect(unread(1, 0)).toBe(true);
  });
});
