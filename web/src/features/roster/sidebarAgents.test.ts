import { describe, expect, it } from "vitest";
import type { Machine, Pane, Session, Tab } from "./api";
import { DEFAULT_RADAR } from "./radar";
import {
  agentKey,
  askWords,
  elbowOf,
  fleetTally,
  livePins,
  pinnedRows,
  shapeOf,
  stepAgent,
  subagentCount,
  subagentsOf,
  syncTip,
  syncWords,
  tallyOf,
  togglePin,
  waitingAgents,
  walkOrder,
} from "./sidebarAgents";

const now = 1_000_000_000_000;
const ago = (minutes: number) => now - minutes * 60_000;

let ids = 0;
const pane = (over: Partial<Pane> = {}): Pane => ({ id: `p${++ids}`, title: "", agent: "claude", cwd: "/x", focused: false, status: "idle", cols: 80, rows: 24, ...over }) as Pane;
const tab = (panes: Pane[]): Tab => ({ id: `t${++ids}`, number: 1, label: "", named: false, status: "idle", panes }) as unknown as Tab;
const session = (name: string, panes: Pane[], over: Partial<Session> = {}): Session => ({ id: `w${++ids}`, name, status: "idle", active_tab_id: "", tabs: [tab(panes)], ...over }) as Session;
const machine = (id: string, sessions: Session[], state: Machine["state"] = "connected"): Machine => ({ id, label: id.toUpperCase(), state, sessions }) as unknown as Machine;

describe("shapeOf", () => {
  it("gives the four shapes that read without colour, and the watchdog's note its own", () => {
    expect(shapeOf(pane({ radar: { state: "working" } }), "working")).toBe("working");
    expect(shapeOf(pane({ radar: { state: "blocked" } }), "blocked")).toBe("asking");
    expect(shapeOf(pane({ radar: { state: "blocked", alert: "waiting 20 min" } }), "blocked")).toBe("asking");
    expect(shapeOf(pane({ radar: { state: "working", alert: "stuck 25 min" } }), "working")).toBe("note");
    expect(shapeOf(pane({ radar: { state: "done" } }), "done")).toBe("done");
    for (const d of ["idle", "idle_fresh", "idle_stale", "unknown"] as const) expect(shapeOf(pane({ radar: { state: "idle" } }), d)).toBe("idle");
  });
});

describe("waitingAgents", () => {
  it("lists every agent that asks or carries a note on a connected machine, the oldest wait first", () => {
    const recent = pane({ radar: { state: "blocked", since: ago(2) } });
    const old = pane({ radar: { state: "working", since: ago(40), alert: "stuck 25 min" } });
    const unstamped = pane({ radar: { state: "blocked" } });
    const away = pane({ radar: { state: "blocked", since: ago(90) } });
    const scratch = pane({ agent: "claude", radar: { state: "blocked", since: ago(99) } });
    const machines = [
      machine("gog", [session("api", [recent, pane({ radar: { state: "working" } })]), session("db", [unstamped]), session("scratch", [scratch], { scratch: true })]),
      machine("mac", [session("web", [old])]),
      machine("pi", [session("x", [away])], "error"),
    ];
    expect(waitingAgents(machines).map((w) => w.key)).toEqual([agentKey("mac", old.id), agentKey("gog", recent.id), agentKey("gog", unstamped.id)]);
  });
});

describe("tallyOf", () => {
  it("counts the agents, the ones that need you — a stuck one among them, not among the working — the working and the finished", () => {
    const sessions = [session("a", [pane({ radar: { state: "working" } }), pane({ radar: { state: "working", alert: "stuck" } }), pane({ agent: "" })]), session("b", [pane({ radar: { state: "blocked" } }), pane({ radar: { state: "done" } })])];
    expect(tallyOf(sessions)).toEqual({ agents: 4, needs: 2, working: 1, done: 1 });
    expect(fleetTally([machine("gog", sessions), machine("pi", sessions, "connecting")])).toEqual({ agents: 4, needs: 2, working: 1, done: 1 });
  });
});

describe("pins", () => {
  const a = pane();
  const b = pane();
  const machines = [machine("gog", [session("api", [a, b])]), machine("pi", [], "error")];

  it("pins at the end and unpins", () => {
    expect(togglePin([], "gog|p1")).toEqual(["gog|p1"]);
    expect(togglePin(["gog|p1", "gog|p2"], "gog|p1")).toEqual(["gog|p2"]);
  });

  it("draws the pinned agents in pin order, the ones the roster shows", () => {
    const rows = pinnedRows(machines, [agentKey("gog", b.id), "pi|w1:p1", agentKey("gog", a.id)]);
    expect(rows.map((r) => r.row.pane)).toEqual([b, a]);
    expect(rows[0]!.row.session.name).toBe("api");
  });

  it("lets go of a pin whose agent is gone from a connected machine, or whose machine the fleet no longer has", () => {
    expect(livePins(machines, [agentKey("gog", a.id), "gog|w9:p9", "pi|w1:p1", "gone|w1:p1"])).toEqual([agentKey("gog", a.id), "pi|w1:p1"]);
    expect(livePins([], ["gog|w9:p9"])).toEqual(["gog|w9:p9"]);
    expect(livePins([machine("gog", [])], ["gog|w9:p9"])).toEqual(["gog|w9:p9"]);
  });
});

describe("subagents", () => {
  it("names the working ones, counts the rest, and draws the last on a rounded elbow", () => {
    const p = pane({ subagents: { working: 4, session: "s1", list: [{ id: "agent-a", description: "explore · cart" }, { id: "agent-b", description: "test-runner" }] } });
    expect(subagentsOf(p)).toEqual({ list: p.subagents!.list, more: 2 });
    expect(subagentCount(p)).toBe(4);
    expect(subagentCount(pane())).toBe(0);
    expect([0, 1, 2].map((i) => elbowOf(i, 3))).toEqual(["├", "├", "└"]);
  });
});

describe("the walk", () => {
  const first = pane({ radar: { state: "idle", last_active: ago(60) } });
  const second = pane({ radar: { state: "working" } });
  const shell = pane({ agent: "" });
  const third = pane({ radar: { state: "done", last_active: ago(5) } });
  const machines = [machine("gog", [session("api", [first]), session("web", [second, shell])], "connected"), machine("mac", [session("cli", [third])])];

  it("goes through every agent in the order drawn, the pinned first, each once, shells left out", () => {
    const order = walkOrder(machines, DEFAULT_RADAR, [agentKey("mac", third.id)]).map((a) => agentKey(a.machineId, a.paneId));
    expect(order).toEqual([agentKey("mac", third.id), agentKey("gog", second.id), agentKey("gog", first.id)]);
  });

  it("steps round and round, from the first when the pane on screen is none of them", () => {
    const order = walkOrder(machines, DEFAULT_RADAR, []);
    const keys = order.map((a) => agentKey(a.machineId, a.paneId));
    const step = (from: string, by: 1 | -1) => {
      const next = stepAgent(order, from, by);
      return next ? agentKey(next.machineId, next.paneId) : null;
    };
    expect(step(keys[0]!, 1)).toBe(keys[1]);
    expect(step(keys[keys.length - 1]!, 1)).toBe(keys[0]);
    expect(step(keys[0]!, -1)).toBe(keys[keys.length - 1]);
    expect(step("", 1)).toBe(keys[0]);
    expect(step("", -1)).toBe(keys[keys.length - 1]);
    expect(stepAgent([], "", 1)).toBeNull();
  });
});

describe("words", () => {
  it("says what an inbox row waits on: the ask, else the note, else an answer", () => {
    expect(askWords(pane({ radar: { state: "blocked" } }), "Bash command · rm -rf build")).toBe("Bash command · rm -rf build");
    expect(askWords(pane({ radar: { state: "blocked" } }), undefined)).toBe("waiting for your answer");
    expect(askWords(pane({ radar: { state: "working", alert: "stuck 25 min" } }), undefined)).toBe("stuck 25 min");
  });

  it("says the distance from the upstream with the zeros left out, nothing when level or unknown", () => {
    expect(syncWords({ ahead: 2, behind: 1 })).toBe("↑2 ↓1");
    expect(syncWords({ ahead: 0, behind: 3 })).toBe("↓3");
    expect(syncWords({ ahead: 0, behind: 0 })).toBe("");
    expect(syncWords(undefined)).toBe("");
    expect(syncTip({ ahead: 2, behind: 0 })).toBe("2 commits not pushed · as of the last fetch");
  });
});
