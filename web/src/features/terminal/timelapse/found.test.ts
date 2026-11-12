import { afterEach, describe, expect, it } from "vitest";
import { setLang } from "@/lib/i18n";
import type { ScreenHit } from "./api";
import { excerptRows, firstStep, momentOf, stayWords, stepAt, stepFrom, stepsOf, stepWords, unreadWords } from "./found";

const hit = (recording: string, over: Partial<ScreenHit> = {}): ScreenHit => ({
  recording,
  machine: "local",
  pane: "w1:p1",
  workspace: "app",
  dir: "/repo/app",
  folder: "/repo",
  agent: "claude",
  title: "",
  start: "2026-11-12T14:00:00.000Z",
  first: { frame: 3, at: 12_000, for: 90_000, line: 7 },
  times: 1,
  before: ["$ make migrate"],
  row: "Migration failed: users",
  after: [],
  col: 0,
  ...over,
});

afterEach(() => setLang("en"));

describe("the steps through the screens", () => {
  it("walks the recordings shown in their order, each by time, and none of a recording not shown", () => {
    const newer = hit("b", { first: { frame: 9, at: 40_000, for: 4000, line: 1 }, again: [{ frame: 2, at: 8000, for: 4000, line: 0 }], times: 2 });
    const older = hit("a");
    const hidden = hit("hidden");
    const steps = stepsOf([older, hidden, newer], ["b", "a"]);
    expect(steps).toEqual([
      { recording: "b", frame: 2, at: 8000, line: 0 },
      { recording: "b", frame: 9, at: 40_000, line: 1 },
      { recording: "a", frame: 3, at: 12_000, line: 7 },
    ]);
    expect(stepAt(steps, { recording: "a", frame: 3 })).toBe(2);
    expect(stepAt(steps, { recording: "a", frame: 4 })).toBe(-1);
    expect(stepAt(steps, null)).toBe(-1);
    expect(firstStep(older)).toEqual({ recording: "a", frame: 3, at: 12_000, line: 7 });
  });

  it("steps round and round, from none to the first or the last", () => {
    expect(stepFrom(3, -1, 1)).toBe(0);
    expect(stepFrom(3, -1, -1)).toBe(2);
    expect(stepFrom(3, 2, 1)).toBe(0);
    expect(stepFrom(3, 0, -1)).toBe(2);
    expect(stepFrom(3, 1, 1)).toBe(2);
    expect(stepFrom(0, -1, 1)).toBe(-1);
  });

  it("counts them", () => {
    expect(stepWords(2, 12)).toBe("3 of 12");
    expect(stepWords(-1, 12)).toBe("12 matches");
    expect(stepWords(-1, 1)).toBe("1 match");
    expect(stepWords(-1, 0)).toBe("0 of 0");
    setLang("ru");
    expect(stepWords(-1, 3)).toBe("3 совпадения");
    expect(stepWords(0, 5)).toBe("1 из 5");
  });
});

describe("a hit's words", () => {
  it("says when the text first stood there, for how long, and how often it came back", () => {
    expect(momentOf(hit("a")).toISOString()).toBe("2026-11-12T14:00:12.000Z");
    expect(stayWords(hit("a"))).toBe("on screen for 1 min 30 s");
    expect(stayWords(hit("a", { times: 4 }))).toBe("on screen for 1 min 30 s · appeared 4 times");
    setLang("ru");
    expect(stayWords(hit("a", { times: 2 }))).toBe("на экране 1 мин 30 с · появлялся 2 раза");
  });

  it("says what a search left unread, and nothing when it read every recording", () => {
    expect(unreadWords({ hits: [], unread: 0, more: false })).toBe("");
    expect(unreadWords({ hits: [], unread: 0, more: false, cut: "time" })).toBe("");
    expect(unreadWords({ hits: [], unread: 5, more: false, cut: "budget" })).toBe("5 older recordings were not searched — narrow it down with a folder or a period.");
    expect(unreadWords({ hits: [hit("a"), hit("b")], unread: 3, more: true })).toMatch(/^Only the newest 2 recordings that show it/);
    setLang("ru");
    expect(unreadWords({ hits: [], unread: 1, more: false, cut: "time" })).toMatch(/^1 более старая запись не просмотрена/);
  });

  it("lays the excerpt out with the matching row marked and a cut on its left shown", () => {
    expect(excerptRows(hit("a"))).toEqual([
      { text: "$ make migrate", match: false },
      { text: "Migration failed: users", match: true },
    ]);
    expect(excerptRows(hit("a", { col: 40, before: ["", "xx"], after: ["yy"] }))).toEqual([
      { text: "", match: false },
      { text: "…xx", match: false },
      { text: "…Migration failed: users", match: true },
      { text: "…yy", match: false },
    ]);
  });
});
