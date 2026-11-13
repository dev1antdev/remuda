import { describe, expect, it } from "vitest";
import type { Pane, Session } from "./api";
import { priorityOf, prioritySections, ticketPrefix } from "./sections";
import type { WorkspaceItem } from "./workspaces";

const pane = (radar?: Pane["radar"], agent = "claude"): Pane => ({ id: "p", title: "", agent, cwd: "/x", focused: false, status: "idle", cols: 80, rows: 24, radar }) as Pane;
const item = (name: string, panes: Pane[], best = 0): WorkspaceItem => {
  const session = { id: name, name, status: "idle", active_tab_id: "", tabs: [] } as unknown as Session;
  const rows = panes.map((p, index) => ({ session, tab: { id: "t" } as never, pane: p, last: 0, head: index === 0, index }));
  return { session, rows, lead: rows[0] ?? null, best };
};

describe("ticketPrefix", () => {
  it("reads a ticket's prefix as munza's better-sidebar does, after a folder too", () => {
    expect(ticketPrefix("PR-12")).toBe("PR");
    expect(ticketPrefix("PLAT-work")).toBe("PLAT");
    expect(ticketPrefix("feature/INS-4-cache")).toBe("INS");
    expect(ticketPrefix("pr-12 fix")).toBe("PR");
    expect(ticketPrefix("herdr-mods")).toBeNull();
    expect(ticketPrefix("main", undefined, "web INS-9")).toBeNull();
    expect(ticketPrefix(undefined, "INS-9")).toBe("INS");
  });
});

describe("priorityOf", () => {
  it("puts a workspace in the section of its most urgent agent, a shell's at rest", () => {
    expect(priorityOf(item("a", [pane({ state: "working" }), pane({ state: "done" })]))).toBe("done");
    expect(priorityOf(item("b", [pane({ state: "done" }), pane({ state: "blocked" })]))).toBe("needs");
    expect(priorityOf(item("c", [pane({ state: "working", alert: "stuck 25 min" })]))).toBe("needs");
    expect(priorityOf(item("d", [pane({ state: "working" })]))).toBe("working");
    expect(priorityOf(item("e", [pane(undefined, "")]))).toBe("idle");
  });

  it("leaves an empty section out and orders a section by activity", () => {
    const sections = prioritySections([item("quiet", [pane({ state: "working" })], 1), item("lively", [pane({ state: "working" })], 9)]);
    expect(sections.map((s) => [s.key, s.items.map((i) => i.session.name)])).toEqual([["priority:working", ["lively", "quiet"]]]);
  });
});
