import { describe, expect, it } from "vitest";
import type { Entry } from "@/features/activity/api";
import { alertOf, keepNewest, MAX_ON_PAGE, newAlerts, wanted } from "./onpage";
import type { Prefs } from "./push";

const entry = (id: number, kind: string, more: Partial<Entry> = {}): Entry => ({ id, at: "", kind, machine: "local", pane: "w1:p1", title: `t${id}`, ...more });
const prefs = (more: Partial<Prefs> = {}): Prefs => ({ input: true, done: "long", watchdog: true, ...more });

describe("wanted", () => {
  it("follows the device's switches as the server does", () => {
    expect(wanted(entry(1, "input"), prefs())).toBe(true);
    expect(wanted(entry(1, "input"), prefs({ input: false }))).toBe(false);
    expect(wanted(entry(1, "done", { worked: 30 }), prefs())).toBe(false);
    expect(wanted(entry(1, "done", { worked: 90 }), prefs())).toBe(true);
    expect(wanted(entry(1, "done", { worked: 3 }), prefs({ done: "always" }))).toBe(true);
    expect(wanted(entry(1, "done", { worked: 900 }), prefs({ done: "off" }))).toBe(false);
    for (const kind of ["stuck", "waiting", "loop", "review", "budget", "limits", "machine", "task", ""]) {
      expect(wanted(entry(1, kind), prefs()), kind).toBe(true);
      expect(wanted(entry(1, kind), prefs({ watchdog: false })), kind).toBe(false);
    }
    expect(wanted(entry(1, "digest"), prefs({ watchdog: false }))).toBe(true);
    expect(wanted(entry(1, "schedule"), prefs({ watchdog: false }))).toBe(true);
  });
  it("never tells the person of their own doing", () => {
    for (const kind of ["sent", "recover", "handover", "teleport"]) expect(wanted(entry(1, kind), prefs()), kind).toBe(false);
    expect(wanted(entry(1, "input", { quiet: true }), prefs())).toBe(false);
  });
});

describe("newAlerts", () => {
  it("is what came after the last seen, oldest first, the newest few of a burst", () => {
    const feed = [entry(9, "input"), entry(8, "sent", { quiet: true }), entry(7, "done", { worked: 120 }), entry(6, "input"), entry(5, "input")];
    expect(newAlerts(feed, 5, prefs()).map((e) => e.id)).toEqual([6, 7, 9]);
    const burst = Array.from({ length: 8 }, (_, i) => entry(20 - i, "input"));
    expect(newAlerts(burst, 0, prefs()).map((e) => e.id)).toEqual([18, 19, 20]);
    expect(newAlerts(feed, 9, prefs())).toEqual([]);
  });
});

describe("alertOf and keepNewest", () => {
  it("makes a toast that opens the pane, a digest the standup", () => {
    expect(alertOf(entry(3, "input", { title: "api needs you", body: "claude is waiting" }))).toEqual({
      id: "alert-3",
      kind: "input",
      text: "api needs you · claude is waiting",
      machine: "local",
      pane: "w1:p1",
      panel: undefined,
      tone: "warn",
      lingers: false,
    });
    expect(alertOf(entry(4, "done"))).toMatchObject({ tone: "success", lingers: true });
    expect(alertOf(entry(5, "digest", { pane: "", machine: "" }))).toMatchObject({ panel: "standup", pane: undefined, machine: undefined });
  });
  it("keeps one of each and the newest few", () => {
    let standing = keepNewest([], alertOf(entry(1, "input")));
    standing = keepNewest(standing, alertOf(entry(1, "input")));
    expect(standing).toHaveLength(1);
    for (let id = 2; id < 8; id++) standing = keepNewest(standing, alertOf(entry(id, "done")));
    expect(standing.map((a) => a.id)).toEqual(Array.from({ length: MAX_ON_PAGE }, (_, i) => `alert-${8 - MAX_ON_PAGE + i}`));
  });
});
