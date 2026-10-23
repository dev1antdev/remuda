import { describe, expect, it } from "vitest";
import type { Machine } from "@/features/roster/api";
import { langOf, linesToSay } from "./voice";

type PaneBits = { id: string; agent?: string; radar?: { state: string; since?: number; alert?: string }; said?: string };
const machine = (id: string, label: string, state: string, panes: PaneBits[]): Machine =>
  ({ id, label, state, sessions: [{ id: "w1", name: "api", tabs: [{ id: "t1", panes: panes.map((p) => ({ agent: "claude", ...p })) }] }] }) as unknown as Machine;

describe("linesToSay", () => {
  it("says a finish with its first line, once per episode", () => {
    const roster = [machine("local", "Mac", "connected", [{ id: "p1", radar: { state: "done", since: 10 }, said: "Tests pass." }])];
    const first = linesToSay(roster, new Set());
    expect(first.lines).toEqual([{ key: "local|p1|done|10", text: "api: Tests pass.", lang: "" }]);
    expect(linesToSay(roster, first.keys).lines).toEqual([]);
    const again = [machine("local", "Mac", "connected", [{ id: "p1", radar: { state: "done", since: 20 }, said: "Tests pass." }])];
    expect(linesToSay(again, first.keys).lines.map((l) => l.key)).toEqual(["local|p1|done|20"]);
  });

  it("waits for the said line before a finish is read", () => {
    expect(linesToSay([machine("local", "Mac", "connected", [{ id: "p1", radar: { state: "done", since: 10 } }])], new Set()).lines).toEqual([]);
  });

  it("names a question, a watchdog note first, and the machine of a remote agent", () => {
    const roster = [
      machine("gog", "GOG", "connected", [
        { id: "p1", radar: { state: "blocked", since: 5 } },
        { id: "p2", radar: { state: "working", alert: "looks stuck" } },
      ]),
    ];
    expect(linesToSay(roster, new Set()).lines.map((l) => l.text)).toEqual(["api on GOG is waiting for you", "api on GOG: looks stuck"]);
  });

  it("skips shells, working agents and machines that are not connected", () => {
    const roster = [
      machine("local", "Mac", "connected", [
        { id: "p1", agent: "", radar: { state: "done" }, said: "x" },
        { id: "p2", radar: { state: "working" }, said: "old" },
      ]),
      machine("roofr", "Roofr", "dead", [{ id: "p1", radar: { state: "done", since: 1 }, said: "Done." }]),
    ];
    expect(linesToSay(roster, new Set())).toEqual({ lines: [], keys: new Set() });
  });

  it("prunes what is gone from the keys to remember", () => {
    const roster = [machine("local", "Mac", "connected", [{ id: "p1", radar: { state: "done", since: 10 }, said: "Done." }])];
    expect([...linesToSay(roster, new Set(["local|p9|done|1"])).keys]).toEqual(["local|p1|done|10"]);
  });
});

describe("langOf", () => {
  it("picks Russian for Cyrillic text", () => {
    expect(langOf("api: Все тесты прошли")).toBe("ru");
    expect(langOf("api: tests pass")).toBe("");
  });
});
