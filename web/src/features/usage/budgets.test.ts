import { describe, expect, it } from "vitest";
import type { BudgetStatus } from "./api";
import { budgetTone, budgetWords, budgetsFor, graver, missingWords, periodWords, ratio, scopeWords, tightest, toneClass } from "./budgets";

const budget = (over: Partial<BudgetStatus> = {}): BudgetStatus => ({
  id: "b",
  limit: 20,
  period: "week",
  spent: 16.4,
  start: "2026-10-19T00:00:00+03:00",
  collected: "2026-10-25T10:40:00Z",
  ...over,
});

describe("ratio and tone", () => {
  it("is spent over limit, nothing while the spend is not read", () => {
    expect(ratio(budget())).toBeCloseTo(0.82);
    expect(ratio(budget({ collected: undefined }))).toBe(0);
    expect(ratio(budget({ limit: 0 }))).toBe(0);
  });
  it("steps at half, 80 % and the limit", () => {
    expect(budgetTone(0.49)).toBe("");
    expect(budgetTone(0.5)).toBe("dim");
    expect(budgetTone(0.8)).toBe("amber");
    expect(budgetTone(1)).toBe("red");
    expect(budgetTone(3)).toBe("red");
  });
  it("picks the graver tone and the pill's class", () => {
    expect(graver("amber", "red")).toBe("red");
    expect(graver("dim", "")).toBe("dim");
    expect(graver("amber", "amber")).toBe("amber");
    expect(toneClass("red")).toBe(" is-red");
    expect(toneClass("amber")).toBe(" is-amber");
    expect(toneClass("dim")).toBe("");
    expect(toneClass("")).toBe("");
  });
});

describe("budgetsFor and tightest", () => {
  const all = budget({ id: "all" });
  const gog = budget({ id: "gog", machine: "gog", spent: 19 });
  const folder = budget({ id: "folder", machine: "gog", dir: "/home/me/remuda", spent: 2, limit: 2 });
  const other = budget({ id: "other", machine: "gog", dir: "/home/me/other" });
  const local = budget({ id: "local", machine: "local" });
  const list = [all, gog, folder, other, local];
  it("keeps the budgets covering a pane", () => {
    expect(budgetsFor(list, "gog", "/home/me/remuda").map((b) => b.id)).toEqual(["all", "gog", "folder"]);
    expect(budgetsFor(list, "gog", "").map((b) => b.id)).toEqual(["all", "gog"]);
    expect(budgetsFor(list, "roofr", "/x").map((b) => b.id)).toEqual(["all"]);
    expect(budgetsFor(undefined, "gog", "/x")).toEqual([]);
  });
  it("names the one furthest along", () => {
    expect(tightest(budgetsFor(list, "gog", "/home/me/remuda"))?.id).toBe("folder");
    expect(tightest(budgetsFor(list, "gog", ""))?.id).toBe("gog");
    expect(tightest([])).toBeNull();
  });
});

describe("words", () => {
  it("says the period as a sentence wants it", () => {
    expect(periodWords("day")).toBe("today");
    expect(periodWords("week")).toBe("this week");
    expect(periodWords("month")).toBe("this month");
  });
  it("writes the spend against the limit", () => {
    expect(budgetWords(budget())).toBe("$16.4 of $20.0 this week · 82 %");
    expect(budgetWords(budget({ spent: 21.3, period: "day" }))).toBe("$21.3 of $20.0 today · 106 %");
    expect(budgetWords(budget({ spent: 0.5, limit: 1, period: "month" }))).toBe("$0.50 of $1.00 this month · 50 %");
    expect(budgetWords(budget({ collected: undefined }))).toBe("$20.0 this week · reading…");
    expect(budgetWords(budget({ spent: 190.5, limit: 0.01, period: "day" }))).toBe("$191 of $0.01 today · far past the limit");
  });
  it("names the machines a reading left out", () => {
    const label = (id: string) => id.toUpperCase();
    expect(missingWords(budget(), label)).toBe("");
    expect(missingWords(budget({ missing: ["gog"] }), label)).toBe("GOG not counted: no report from it");
    expect(missingWords(budget({ missing: ["gog", "roofr"] }), label)).toBe("GOG, ROOFR not counted: no report from them");
  });
  it("names the scope", () => {
    const label = (id: string) => id.toUpperCase();
    expect(scopeWords(budget(), label)).toBe("everything");
    expect(scopeWords(budget({ machine: "gog" }), label)).toBe("GOG");
    expect(scopeWords(budget({ machine: "gog", dir: "/home/me/remuda" }), label)).toBe("GOG · remuda");
  });
});
