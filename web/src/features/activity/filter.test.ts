import { describe, expect, it } from "vitest";
import type { Entry } from "./api";
import { DEFAULT_FILTER, counts, familyOf, filterEntries, filterWords, isFiltering, toggleFamily } from "./filter";

const entry = (id: number, kind: string, machine = "local"): Entry => ({ id, at: "2026-10-29T10:00:00", kind, machine, title: `e${id}` });
const feed = [entry(1, "input"), entry(2, "done", "gog"), entry(3, "stuck"), entry(4, "task", "gog"), entry(5, "handover"), entry(6, "sent"), entry(7, "machine", "gog"), entry(8, "something-new")];

describe("familyOf", () => {
  it("gathers the feed's kinds into chips and puts the unknown with the other", () => {
    expect(familyOf("input")).toBe("questions");
    expect(familyOf("done")).toBe("finished");
    expect(familyOf("loop")).toBe("watchdog");
    expect(familyOf("digest")).toBe("schedules");
    expect(familyOf("sent")).toBe("sent");
    expect(familyOf("machine")).toBe("machines");
    expect(familyOf("handover")).toBe("other");
    expect(familyOf("something-new")).toBe("other");
  });
});

describe("filterEntries", () => {
  it("shows everything with no filter and narrows by family and machine", () => {
    expect(filterEntries(feed, DEFAULT_FILTER)).toHaveLength(8);
    expect(filterEntries(feed, { kinds: ["questions", "tasks"], machine: "all" }).map((e) => e.id)).toEqual([1, 4]);
    expect(filterEntries(feed, { kinds: [], machine: "gog" }).map((e) => e.id)).toEqual([2, 4, 7]);
    expect(filterEntries(feed, { kinds: ["other"], machine: "local" }).map((e) => e.id)).toEqual([5, 8]);
    expect(filterEntries(feed, { kinds: ["review"], machine: "all" })).toEqual([]);
  });
});

describe("counts and toggles", () => {
  it("counts per family within the machine chosen", () => {
    expect(counts(feed, "all")).toEqual({ questions: 1, finished: 1, watchdog: 1, tasks: 1, other: 2, sent: 1, machines: 1 });
    expect(counts(feed, "gog")).toEqual({ finished: 1, tasks: 1, machines: 1 });
  });
  it("toggles a chip and knows when anything is filtered", () => {
    const on = toggleFamily(DEFAULT_FILTER, "tasks");
    expect(on.kinds).toEqual(["tasks"]);
    expect(toggleFamily(on, "tasks").kinds).toEqual([]);
    expect(isFiltering(DEFAULT_FILTER)).toBe(false);
    expect(isFiltering(on)).toBe(true);
    expect(isFiltering({ kinds: [], machine: "gog" })).toBe(true);
  });
  it("says what is on for the bell", () => {
    const label = (f: { label: string }) => f.label.toLowerCase();
    expect(filterWords(DEFAULT_FILTER, label, "GOG")).toBe("");
    expect(filterWords({ kinds: ["tasks", "questions"], machine: "all" }, label, "GOG")).toBe("questions, tasks");
    expect(filterWords({ kinds: ["sent"], machine: "gog" }, label, "GOG")).toBe("sent · GOG");
  });
});
