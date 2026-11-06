import { describe, expect, it } from "vitest";
import type { Entry } from "@/features/activity/api";
import { alertOf, alertText, keepNewest, MAX_ON_PAGE, newAlerts, quietNow, stillStanding, wanted } from "./onpage";
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
    expect(wanted(entry(1, "backup", { machine: "", pane: "" }), prefs({ watchdog: false })), "a failed backup goes to every device, as push.Broadcast sends it").toBe(true);
    expect(alertOf(entry(1, "backup", { machine: "", pane: "" })).tone).toBe("warn");
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
    const question = alertOf(entry(3, "input", { title: "api needs you", body: "claude is waiting" }));
    expect(question).toEqual({
      id: "alert-3",
      kind: "input",
      title: "api needs you",
      body: "claude is waiting",
      machine: "local",
      pane: "w1:p1",
      panel: undefined,
      tone: "warn",
      lingers: false,
    });
    expect(alertText(question)).toBe("api needs you · claude is waiting");
    expect(alertText(alertOf(entry(9, "machine", { pane: "" })))).toBe("t9");
    expect(alertOf(entry(4, "done"))).toMatchObject({ tone: "success", lingers: true });
    expect(alertOf(entry(5, "digest", { pane: "", machine: "" }))).toMatchObject({ panel: "standup", pane: undefined, machine: undefined });
  });
  it("opens the usage for a budget or a limit, and a backup's failure warns", () => {
    expect(alertOf(entry(6, "budget", { pane: "" }))).toMatchObject({ panel: "usage", tone: "warn" });
    expect(alertOf(entry(7, "limits", { pane: "" }))).toMatchObject({ panel: "usage" });
    expect(alertOf(entry(8, "limits"))).toMatchObject({ panel: undefined, pane: "w1:p1" });
    expect(alertOf(entry(10, "backup", { pane: "" }))).toMatchObject({ panel: undefined, tone: "warn" });
  });
  it("keeps one of each and the newest few", () => {
    let standing = keepNewest([], alertOf(entry(1, "input")));
    standing = keepNewest(standing, alertOf(entry(1, "input")));
    expect(standing).toHaveLength(1);
    for (let id = 2; id < 8; id++) standing = keepNewest(standing, alertOf(entry(id, "done")));
    expect(standing.map((a) => a.id)).toEqual(Array.from({ length: MAX_ON_PAGE }, (_, i) => `alert-${8 - MAX_ON_PAGE + i}`));
  });
});

describe("what the server broadcasts", () => {
  it("shows a backup's failure whatever the watchdog switch, as push.Broadcast sends it", () => {
    expect(wanted(entry(1, "backup", { pane: "" }), prefs({ watchdog: false }))).toBe(true);
  });
});

describe("stillStanding", () => {
  it("takes a question's toast down once its pane no longer asks, and nothing else", () => {
    const standing = stillStanding([alertOf(entry(1, "input")), alertOf(entry(2, "done")), alertOf(entry(3, "input", { pane: "w2:p1" }))], () => true);
    expect(stillStanding(standing, () => true)).toBe(standing);
    const kept = stillStanding(standing, (_machine, pane) => pane === "w2:p1");
    expect(kept.map((a) => a.id)).toEqual(["alert-2", "alert-3"]);
    expect(stillStanding([alertOf(entry(4, "input", { pane: "" }))], () => false)).toHaveLength(1);
  });
  it("keeps a note about a pane that never asked — a first prompt not delivered — for as long as any other alert", () => {
    const note = alertOf(entry(5, "input", { title: "The first prompt for api was not delivered" }));
    const shown = stillStanding([note], () => false);
    expect(shown).toMatchObject([{ id: "alert-5", lingers: true }]);
    expect(stillStanding(shown, () => false)).toBe(shown);
  });
});

describe("quietNow", () => {
  const at = (iso: string) => new Date(iso);
  it("reads the window in the device's own zone, as the server's Quiet.Covers", () => {
    const night = { from: "22:00", to: "07:00", tz: "Europe/Warsaw" };
    expect(quietNow(night, at("2026-11-05T22:30:00+01:00"))).toBe(true);
    expect(quietNow(night, at("2026-11-05T06:59:00+01:00"))).toBe(true);
    expect(quietNow(night, at("2026-11-05T07:00:00+01:00"))).toBe(false);
    expect(quietNow(night, at("2026-11-05T12:00:00+01:00"))).toBe(false);
    expect(quietNow({ from: "09:00", to: "17:00", tz: "UTC" }, at("2026-11-05T12:00:00Z"))).toBe(true);
    expect(quietNow({ from: "09:00", to: "09:00", tz: "UTC" }, at("2026-11-05T09:00:00Z"))).toBe(false);
    expect(quietNow({ from: "09:00", to: "17:00", tz: "Not/AZone" }, at("2026-11-05T12:00:00Z"))).toBe(true);
    expect(quietNow(undefined)).toBe(false);
  });
});
