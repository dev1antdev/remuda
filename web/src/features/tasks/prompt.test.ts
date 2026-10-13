import { describe, expect, it } from "vitest";
import { branchFor, dueLabel, endOfToday, toLocalInput } from "./prompt";

describe("branchFor", () => {
  it("slugs the title after the id", () => {
    expect(branchFor({ id: 12, title: "Fix the Login Page!" })).toBe("task-12-fix-the-login-page");
    expect(branchFor({ id: 3, title: "   " })).toBe("task-3");
    expect(branchFor({ id: 7, title: "Ünïcode wörds are kept" })).toBe("task-7-ünïcode-wörds-are-kept");
  });
  it("cuts a long title at forty characters, never on a dash", () => {
    const slug = branchFor({ id: 1, title: "a very long title that goes on and on and on and on forever" }).slice("task-1-".length);
    expect(slug.length).toBeLessThanOrEqual(40);
    expect(slug.endsWith("-")).toBe(false);
  });
});

describe("dueLabel", () => {
  const now = new Date(2026, 9, 13, 12, 0, 0);
  const at = (d: Date) => ({ due_date: d.toISOString() });
  it("says today, tomorrow, yesterday, a weekday within the week, else a date", () => {
    expect(dueLabel(at(new Date(2026, 9, 13, 18, 0)), now)?.text.startsWith("Today")).toBe(true);
    expect(dueLabel(at(new Date(2026, 9, 14, 0, 0)), now)?.text).toBe("Tomorrow");
    expect(dueLabel(at(new Date(2026, 9, 12, 0, 0)), now)?.text).toBe("Yesterday");
    expect(dueLabel(at(new Date(2026, 9, 16, 0, 0)), now)?.text).toBe(new Date(2026, 9, 16).toLocaleDateString([], { weekday: "short" }));
    expect(dueLabel(at(new Date(2026, 10, 2, 0, 0)), now)?.text).toBe(new Date(2026, 10, 2).toLocaleDateString([], { day: "numeric", month: "short" }));
  });
  it("marks what is past", () => {
    expect(dueLabel(at(new Date(2026, 9, 13, 9, 0)), now)?.over).toBe(true);
    expect(dueLabel(at(new Date(2026, 9, 13, 18, 0)), now)?.over).toBe(false);
    expect(dueLabel({ due_date: "" }, now)).toBeNull();
  });
});

describe("dates for the fields", () => {
  it("toLocalInput is the local wall-clock for a datetime-local field", () => {
    expect(toLocalInput(new Date(2026, 9, 13, 18, 5).toISOString())).toBe("2026-10-13T18:05");
    expect(toLocalInput("")).toBe("");
  });
  it("endOfToday is six in the evening, local", () => {
    const end = new Date(endOfToday(new Date(2026, 9, 13, 9, 30)));
    expect([end.getFullYear(), end.getMonth(), end.getDate(), end.getHours(), end.getMinutes()]).toEqual([2026, 9, 13, 18, 0]);
  });
});
