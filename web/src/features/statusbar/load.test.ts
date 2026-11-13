import { describe, expect, it } from "vitest";
import type { Session } from "@/features/roster/api";
import { cpuBarWords, cpuLevel, cpuWords, footWords, graver, memLevel, memWords, paneToOpen, terminalName, workspaceLine, type Usage } from "./load";

const now = Date.parse("2026-11-13T12:00:00Z");

const usage: Usage = {
  at: new Date(now).toISOString(),
  cores: 8,
  mem_kb: 16 * 1024 * 1024,
  total: { cpu: 12.4, rss_kb: 3.8 * 1024 * 1024 },
  cpu_known: true,
  workspaces: [{ id: "w1", cpu: 9, rss_kb: 3 * 1024 * 1024, top: { name: "node", rss_kb: 512 * 1024 }, terminals: [{ pane: "w1:p1", cpu: 9, rss_kb: 1 }, { pane: "w1:p2", cpu: 0, rss_kb: 1 }, { pane: "w1:p3", cpu: 0, rss_kb: 1 }] }],
  herdr: { cpu: 0.2, rss_kb: 40960 },
};

describe("the machine's load", () => {
  it("turns amber and red where Orca does", () => {
    expect([cpuLevel(29), cpuLevel(30), cpuLevel(60)]).toEqual(["", "amber", "red"]);
    expect([memLevel(29, 100), memLevel(30, 100), memLevel(50, 100), memLevel(99, 0)]).toEqual(["", "amber", "red", ""]);
    expect(graver("amber", "")).toBe("amber");
    expect(graver("amber", "red")).toBe("red");
  });

  it("says the CPU and the memory in a few characters", () => {
    expect(cpuWords(0.01)).toBe("0 %");
    expect(cpuWords(4.24)).toBe("4.2 %");
    expect(cpuWords(12.4)).toBe("12 %");
    expect(memWords(512 * 1024)).toBe("512 MB");
    expect(memWords(3.8 * 1024 * 1024)).toBe("3.8 GB");
    expect(cpuBarWords(usage)).toBe("CPU 12 %");
    expect(cpuBarWords({ ...usage, cpu_known: false })).toBe("CPU …");
    expect(workspaceLine(usage.workspaces[0]!)).toBe("node 512 MB · 3 terminals");
    expect(footWords(usage)).toBe("CPU is a share of all 8 cores · 16.0 GB");
  });

  it("names a terminal by what runs in it and its heaviest process", () => {
    const session = { tabs: [{ panes: [{ id: "w1:p1", agent: "Claude Code" }, { id: "w1:p2", agent: "" }] }] } as unknown as Session;
    expect(terminalName(session, { pane: "w1:p1", cpu: 0, rss_kb: 1, top: { name: "node", rss_kb: 1 } })).toBe("Claude Code · node");
    expect(terminalName(session, { pane: "w1:p2", cpu: 0, rss_kb: 1, top: { name: "zsh", rss_kb: 1 } })).toBe("terminal · zsh");
    expect(terminalName(undefined, { pane: "w9:p1", cpu: 0, rss_kb: 1 })).toBe("terminal");
  });

  it("opens a workspace on an agent of its active tab, else any agent, else its first pane", () => {
    const pane = (id: string, agent = "") => ({ id, agent }) as Session["tabs"][number]["panes"][number];
    const session = (active: string, tabs: [string, ReturnType<typeof pane>[]][]) => ({ id: "w1", active_tab_id: active, tabs: tabs.map(([id, panes]) => ({ id, panes })) }) as unknown as Session;
    expect(paneToOpen(session("t2", [["t1", [pane("p1", "claude")]], ["t2", [pane("p2"), pane("p3", "codex")]]]))).toBe("p3");
    expect(paneToOpen(session("t2", [["t1", [pane("p1", "claude")]], ["t2", [pane("p2")]]]))).toBe("p1");
    expect(paneToOpen(session("t2", [["t1", [pane("p1")]], ["t2", [pane("p2")]]]))).toBe("p2");
    expect(paneToOpen(session("t9", []))).toBeUndefined();
  });
});
