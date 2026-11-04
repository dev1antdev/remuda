import { describe, expect, it } from "vitest";
import type { LimitWindow, Machine } from "@/features/roster/api";
import { accountWindows, claudeUnread, gravest, limitTone, limitWords, resetForHit, resetTime } from "./limits";

const now = Date.parse("2026-11-04T10:00:00Z");
const win = (over: Partial<LimitWindow>): LimitWindow => ({ agent: "claude", name: "5h", used: 10, resets_at: "2026-11-04T14:00:00Z", at: "2026-11-04T09:00:00Z", ...over });
const machine = (id: string, windows: LimitWindow[], over: Partial<Machine> = {}): Machine => ({ id, label: id.toUpperCase(), state: "connected", sessions: [], limits: { windows }, ...over }) as Machine;

describe("accountWindows", () => {
  it("keeps the freshest reading of each window across machines, the reset ones out, Claude first and the 5 h first", () => {
    const gog = machine("gog", [win({ used: 40, at: "2026-11-04T09:00:00Z" }), win({ agent: "codex", name: "week", used: 5, resets_at: "2026-11-09T00:00:00Z" })]);
    const roofr = machine("roofr", [win({ used: 62, at: "2026-11-04T09:30:00Z" }), win({ name: "week", used: 20, resets_at: "2026-11-08T00:00:00Z" }), win({ agent: "codex", name: "5h", used: 99, resets_at: "2026-11-04T09:00:00Z" })]);
    const list = accountWindows([gog, roofr], now);
    expect(list.map((w) => `${w.agent} ${w.name} ${w.used} ${w.machine}`)).toEqual(["claude 5h 62 roofr", "claude week 20 roofr", "codex week 5 gog"]);
    expect(list[0]!.machineLabel).toBe("ROOFR");
    expect(accountWindows([machine("x", [])], now)).toEqual([]);
  });
});

describe("the words and tones", () => {
  it("colours from 80 % and at the limit", () => {
    expect(limitTone(79.9)).toBe("");
    expect(limitTone(80)).toBe("amber");
    expect(limitTone(100)).toBe("red");
    expect(gravest([win({ used: 20 }), win({ used: 85 })])).toBe("amber");
    expect(gravest([])).toBe("");
  });

  it("says a window in a line, the reset by time today and by weekday later", () => {
    const today = new Date(now);
    today.setHours(14, 0, 0, 0);
    const time = today.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
    expect(limitWords(win({ used: 62.7, resets_at: today.toISOString() }), now)).toBe(`Claude 5 h: 62 %, resets ${time}`);
    const later = new Date(now + 3 * 86_400_000);
    expect(resetTime(later.toISOString(), now)).toBe(`${later.toLocaleDateString(undefined, { weekday: "short" })} ${later.toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" })}`);
  });
});

describe("resetForHit", () => {
  it("is the earliest reset of the agent's windows at the limit, else near it, else none", () => {
    const windows = [win({ used: 100, resets_at: "2026-11-08T00:00:00Z", name: "week" }), win({ used: 100, resets_at: "2026-11-04T14:00:00Z" }), win({ agent: "codex", used: 100, resets_at: "2026-11-04T11:00:00Z" })];
    expect(resetForHit(windows, "claude")).toBe("2026-11-04T14:00:00Z");
    expect(resetForHit([win({ used: 96, resets_at: "2026-11-05T00:00:00Z" })], "claude")).toBe("2026-11-05T00:00:00Z");
    expect(resetForHit([win({ used: 50 })], "claude")).toBeUndefined();
  });
});

describe("claudeUnread", () => {
  it("is a machine running Claude Code with no wrapper before its status line", () => {
    expect(claudeUnread(machine("a", [], { health: { claude: "2.1.318" } } as Partial<Machine>))).toBe(true);
    expect(claudeUnread(machine("a", [], { health: { claude: "2.1.318", statusline: true } } as Partial<Machine>))).toBe(false);
    expect(claudeUnread(machine("a", [], { health: {} } as Partial<Machine>))).toBe(false);
  });
});
