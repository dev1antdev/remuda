import { describe, expect, it } from "vitest";
import type { Machine, PRStatus } from "@/features/roster/api";
import type { Run } from "./api";
import { paneOf, runPR } from "./runPR";

const status: PRStatus = { state: "OPEN", draft: false, mergeable: "MERGEABLE", review: "", checks: { total: 2, pass: 2, fail: 0, pending: 0, skipped: 0 }, at: "2026-10-27T12:00:00Z" };
const pane = (id: string, pr?: { url: string; label: string; status?: PRStatus }) => ({ id, title: "", agent: "claude", cwd: "/w", focused: false, status: "idle" as const, cols: 80, rows: 24, pr });
const machines: Machine[] = [
  { id: "local", label: "This Mac", state: "connected", sessions: [{ id: "w1", name: "a", status: "idle", active_tab_id: "t1", tabs: [{ id: "t1", label: "1", number: 1, named: false, status: "idle", panes: [pane("w1:p1", { url: "https://github.com/o/r/pull/15", label: "PR #15", status })] }] }] },
  { id: "gog", label: "GOG", state: "connected", sessions: [{ id: "w2", name: "b", status: "idle", active_tab_id: "t2", tabs: [{ id: "t2", label: "1", number: 1, named: false, status: "idle", panes: [pane("w2:p1")] }] }] },
];
const run = (fields: Partial<Run>): Run => ({ id: 1, task_id: 1, machine: "local", workspace: "w1", pane: "w1:p1", dir: "/w", branch: "task-1", agent: "claude", state: "finished", started: "", ...fields });

describe("paneOf", () => {
  it("finds a pane by machine and id, null when either is gone", () => {
    expect(paneOf(machines, "local", "w1:p1")?.id).toBe("w1:p1");
    expect(paneOf(machines, "gog", "w2:p1")?.pr).toBeUndefined();
    expect(paneOf(machines, "gog", "w1:p1")).toBeNull();
    expect(paneOf(machines, "roofr", "w1:p1")).toBeNull();
  });
});

describe("runPR", () => {
  it("is nothing while the run named no request", () => {
    expect(runPR(run({}), paneOf(machines, "local", "w1:p1"))).toBeNull();
    expect(runPR(null, null)).toBeNull();
  });
  it("carries the pane's state while the pane still names the same link", () => {
    const pr = runPR(run({ pr_url: "https://github.com/o/r/pull/15", pr_label: "PR #15" }), paneOf(machines, "local", "w1:p1"));
    expect(pr).toEqual({ url: "https://github.com/o/r/pull/15", label: "PR #15", status });
  });
  it("keeps the link without a state once the pane is gone or names another", () => {
    expect(runPR(run({ pr_url: "https://github.com/o/r/pull/15", pr_label: "PR #15" }), null)).toEqual({ url: "https://github.com/o/r/pull/15", label: "PR #15", status: null });
    expect(runPR(run({ pr_url: "https://github.com/o/r/pull/14", pr_label: "" }), paneOf(machines, "local", "w1:p1"))).toEqual({ url: "https://github.com/o/r/pull/14", label: "PR", status: null });
  });
});
