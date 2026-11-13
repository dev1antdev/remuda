import { describe, expect, it } from "vitest";
import type { UsageYear } from "./api";
import { cellAt, datesOf, dayWords, heatGrid, hiddenFolder, lengthWords, readingWords, streaks, yearView } from "./heatmap";
import { DEFAULT_SHARING, type SharingPref } from "@/features/roster/sharing";

const none = () => false;
const YEAR: UsageYear = {
  from: "2025-11-14",
  to: "2026-11-13",
  zone: "Europe/Kyiv",
  names: { "claude-opus-5-5": "Opus 5.5", "claude-fable-5-1": "Fable 5.1" },
  machines: [
    {
      id: "local",
      label: "This Mac",
      today_at: "2026-11-13T12:00:00Z",
      folders: [
        { project: "-w-app", dir: "/w/app", sessions: 3, longest: 7200, models: { "claude-opus-5-5": 900, "claude-fable-5-1": 100 } },
        { project: "-w-secret", dir: "/w/secret", sessions: 9, longest: 99_000, models: { "claude-fable-5-1": 5000 } },
      ],
      days: [
        { date: "2026-11-11", cells: [{ f: 0, c: 2, t: 500, n: 10 }] },
        { date: "2026-11-12", cells: [{ f: 0, c: 1, t: 300, n: 4 }, { f: 1, c: 9, t: 5000, n: 40 }] },
        { date: "2026-11-13", cells: [{ f: 1, c: 1, t: 100, n: 1 }] },
      ],
    },
    { id: "gog", label: "GOG", reading: 120, away: true, error: "ssh: connection refused", folders: [{ project: "-h-api", dir: "/h/api", sessions: 1, longest: 60, models: { "claude-opus-5-5": 50 } }], days: [{ date: "2026-11-12", cells: [{ f: 0, c: 0.5, t: 50, n: 2 }] }] },
  ],
};

describe("yearView", () => {
  it("adds every machine's folders up day by day, with the year's figures", () => {
    const view = yearView(YEAR, "all", none, "cost");
    expect(view.days.get("2026-11-12")).toEqual({ date: "2026-11-12", cost: 10.5, tokens: 5350, turns: 46 });
    expect(view.figures).toMatchObject({ favourite: "Fable 5.1", tokens: 5950, sessions: 13, longest: 99_000, active: 3, span: 365, current: 3, best: 3 });
    expect(view.figures.busiest?.date).toBe("2026-11-12");
    expect(view).toMatchObject({ reading: 120, away: ["GOG"], failed: [{ label: "GOG", error: "ssh: connection refused" }], readAt: "2026-11-13T12:00:00Z", hidden: 0 });
  });

  it("follows the panel's machine and leaves a hidden folder out of everything", () => {
    const one = yearView(YEAR, "gog", none, "turns");
    expect([...one.days.keys()]).toEqual(["2026-11-12"]);
    expect(one.figures).toMatchObject({ favourite: "Opus 5.5", sessions: 1, active: 1, current: 1 }); // yesterday holds the streak while today has nothing yet
    const shared = yearView(YEAR, "local", (_machine, dir) => dir === "/w/secret", "cost");
    expect(shared.hidden).toBe(1);
    expect(shared.days.get("2026-11-12")).toEqual({ date: "2026-11-12", cost: 1, tokens: 300, turns: 4 });
    expect(shared.days.has("2026-11-13")).toBe(false);
    expect(shared.figures).toMatchObject({ favourite: "Opus 5.5", tokens: 800, sessions: 3, longest: 7200, active: 2, current: 2 });
  });
});

describe("hiddenFolder", () => {
  const on = (more: Partial<SharingPref>): SharingPref => ({ ...DEFAULT_SHARING, on: true, ...more });
  const sessions = [{ dir: "/w/app-wt", folder: "/w/app" }];
  it("hides nothing while the mode is off, everything of a hidden machine or with all", () => {
    expect(hiddenFolder({ ...DEFAULT_SHARING, folders: ["local|/w/app"] }, [], "local", "/w/app")).toBe(false);
    expect(hiddenFolder(on({ all: true }), [], "local", "/w/x")).toBe(true);
    expect(hiddenFolder(on({ machines: ["gog"] }), [], "gog", "/h/api")).toBe(true);
    expect(hiddenFolder(on({ machines: ["gog"] }), [], "local", "/h/api")).toBe(false);
  });
  it("hides a marked folder, a folder inside it, a workspace of it elsewhere and a herdr worktree of it", () => {
    const pref = on({ folders: ["local|/w/app"] });
    expect(hiddenFolder(pref, sessions, "local", "/w/app")).toBe(true);
    expect(hiddenFolder(pref, sessions, "local", "/w/app/web")).toBe(true);
    expect(hiddenFolder(pref, sessions, "local", "/w/app-wt/src")).toBe(true);
    expect(hiddenFolder(pref, sessions, "local", "/Users/me/.herdr/worktrees/app/task-12")).toBe(true);
    expect(hiddenFolder(pref, sessions, "local", "/w/application")).toBe(false);
    expect(hiddenFolder(pref, sessions, "gog", "/w/app")).toBe(false);
  });
});

describe("heatGrid", () => {
  it("lays the year a column a week from Monday, levels by the quartiles of the busy days", () => {
    // 2026-11-02 is a Monday, the 13th a Friday
    const values: Record<string, number> = { "2026-11-02": 1, "2026-11-03": 2, "2026-11-04": 3, "2026-11-05": 4, "2026-11-12": 100 };
    const grid = heatGrid("2026-11-01", "2026-11-13", (d) => values[d] ?? 0);
    expect(grid.cells[0]).toMatchObject({ date: "2026-11-01", col: 0, row: 6, level: 0 });
    expect(grid.cells[1]).toMatchObject({ date: "2026-11-02", col: 1, row: 0, level: 1 });
    expect(grid.cells.find((c) => c.date === "2026-11-12")).toMatchObject({ col: 2, row: 3, level: 4 });
    expect(grid.weeks).toBe(3);
    expect(cellAt(grid, 2, 4)?.date).toBe("2026-11-13");
    expect(cellAt(grid, 2, 5)).toBeNull();
    expect(cellAt(grid, 0, 0)).toBeNull();
  });
  it("names each month where it begins, the earlier of two too close together left out", () => {
    const grid = heatGrid("2025-11-14", "2026-11-13", () => 0);
    expect(grid.weeks).toBe(53);
    expect(grid.months[0]!.date.slice(0, 7)).toBe("2025-11"); // three columns before December: room for its name
    expect(grid.months.map((m) => m.date.slice(5, 7))).toEqual(["11", "12", "01", "02", "03", "04", "05", "06", "07", "08", "09", "10", "11"]);
    expect(heatGrid("2025-11-29", "2026-01-10", () => 0).months.map((m) => m.date.slice(0, 7))).toEqual(["2025-12", "2026-01"]);
    expect(grid.cells.every((c) => c.level === 0)).toBe(true);
  });
});

describe("the year's words", () => {
  it("counts streaks to today, or to yesterday while today is still empty", () => {
    const dates = datesOf("2026-11-01", "2026-11-10");
    expect(dates.length).toBe(10);
    expect(streaks(dates, new Set(["2026-11-02", "2026-11-03", "2026-11-04", "2026-11-08", "2026-11-09"]))).toEqual({ current: 2, best: 3 });
    expect(streaks(dates, new Set(["2026-11-09", "2026-11-10"]))).toEqual({ current: 2, best: 2 });
    expect(streaks(dates, new Set(["2026-11-05"]))).toEqual({ current: 0, best: 1 });
  });
  it("says a day, a session's length and what is still being read", () => {
    expect(dayWords("2026-11-12", { date: "2026-11-12", cost: 4.2, tokens: 1, turns: 312 })).toBe("Thu, Nov 12, 2026 · $4.20 · 312 responses");
    expect(dayWords("2026-11-12", undefined)).toBe("Thu, Nov 12, 2026 · nothing");
    expect(lengthWords(40)).toBe("40 s");
    expect(lengthWords(5 * 3600 + 12 * 60)).toBe("5 h 12 min");
    expect(lengthWords(4 * 86400 + 23 * 3600)).toBe("4 d 23 h");
    expect(lengthWords(2 * 86400)).toBe("2 days");
    expect(readingWords(365)).toBe("Reading 365 days…");
  });
});
