import { describe, expect, it } from "vitest";
import type { Machine } from "@/features/roster/api";
import { shownPane, type Located } from "./shownPane";

const pane = (id: string, agent = "") => ({ id, agent, cwd: "/tmp", title: "", status: "idle", focused: false, cols: 80, rows: 24 });
const session = (id: string, panes: ReturnType<typeof pane>[], scratch = false) => ({ id, name: id, dir: `/tmp/${id}`, scratch, tabs: [{ id: `${id}:t1`, label: "1", number: 1, named: false, status: "idle", panes }] });
const roster = (sessions: ReturnType<typeof session>[]): Located[] => {
  const machine = { id: "local", label: "This Mac", state: "connected", sessions } as unknown as Machine;
  return machine.sessions.flatMap((s) => s.tabs.flatMap((tab) => tab.panes.map((p) => ({ machine, session: s, tab, pane: p }))));
};

describe("shownPane", () => {
  it("is the pane asked for while the roster has it, the floating terminal's own shell too", () => {
    const panes = roster([session("scratch", [pane("w1:p1", "claude")], true), session("api", [pane("w2:p1", "codex")])]);
    expect(shownPane(panes, { machineId: "local", paneId: "w2:p1" })?.pane.id).toBe("w2:p1");
    expect(shownPane(panes, { machineId: "local", paneId: "w1:p1" })?.pane.id).toBe("w1:p1");
  });
  it("falls back to the first agent pane, never one in the floating terminal's shell where an agent was started", () => {
    const panes = roster([session("scratch", [pane("w1:p1", "claude")], true), session("web", [pane("w3:p1")]), session("api", [pane("w2:p1", "codex")])]);
    expect(shownPane(panes, null)?.pane.id).toBe("w2:p1");
    expect(shownPane(panes, { machineId: "local", paneId: "w9:p9" })?.pane.id).toBe("w2:p1");
  });
  it("falls back to the first pane that is not the drawer's when no agent runs, and to nothing on an empty roster", () => {
    expect(shownPane(roster([session("scratch", [pane("w1:p1", "claude")], true), session("web", [pane("w3:p1")])]), null)?.pane.id).toBe("w3:p1");
    expect(shownPane(roster([session("scratch", [pane("w1:p1", "claude")], true)]), null)).toBeNull();
    expect(shownPane([], null)).toBeNull();
  });
});
