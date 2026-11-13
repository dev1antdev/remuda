import { describe, expect, it } from "vitest";
import type { Machine, Pane } from "@/features/roster/api";
import { baseChoices, baseWords, busyAgent, isSubfolder, outgoingTitle, pullWords } from "./scope";

const pane = (agent: string, state: "working" | "blocked" | "idle"): Pane => ({ id: "w1:p1", title: "", agent, cwd: "", focused: false, status: "idle", cols: 80, rows: 24, radar: { state } });
const machine = (dir: string, panes: Pane[]): Machine => ({ id: "local", label: "Mac", state: "connected", sessions: [{ id: "w1", name: "app", status: "idle", active_tab_id: "t1", dir, tabs: [{ id: "t1", label: "", number: 1, named: false, status: "idle", panes }] }] });
const label = (kind: string) => (kind === "codex" ? "Codex" : kind);

describe("the scope", () => {
  it("names the remote a commit is not on yet, and what a pull would bring", () => {
    expect(outgoingTitle({ against: "origin/feature", commits: [], total: 2 })).toBe("Committed — not on origin/feature");
    expect(pullWords({ against: "origin/feature", upstream: true, commits: [], total: 2, behind: 3 })).toBe("3 to pull");
    expect(pullWords({ against: "origin/main", commits: [], total: 2, behind: 3 })).toBe(""); // no upstream: nothing to pull from
    expect(pullWords(undefined)).toBe("");
  });

  it("says against what the branch's work is read", () => {
    expect(baseWords(null)).toBe("Finding the branch it was cut from…");
    expect(baseWords({ base: "", detected: "", ahead: 0, refs: [], files: [] })).toBe("No branch to compare with");
    expect(baseWords({ base: "origin/main", detected: "origin/main", merge_base: "0123456789abcdef", ahead: 1, refs: [], files: [] })).toBe("vs origin/main · 1 commit ahead · since 0123456");
    expect(baseChoices({ base: "main", detected: "feature", ahead: 2, refs: ["main", "feature"], files: [] }).map((choice) => choice.value)).toEqual(["", "main", "feature"]);
  });

  it("tells a folder inside the repository from its top, a real path and all", () => {
    expect([isSubfolder("/repo/app", "/repo/app"), isSubfolder("/private/var/t/app", "/var/t/app"), isSubfolder("/repo/app", "/repo/app/web"), isSubfolder(undefined, "/x")]).toEqual([false, false, true, false]);
  });

  it("finds the agent at work or asking in the checkout", () => {
    expect(busyAgent(machine("/repo/app", [pane("codex", "working")]), "/repo/app", "/repo/app", label)).toBe("Codex");
    expect(busyAgent(machine("/repo/app/sub", [pane("claude", "blocked")]), "/repo/app", "/repo/app", label)).toBe("claude");
    expect(busyAgent(machine("/repo/app", [pane("codex", "idle")]), "/repo/app", "/repo/app", label)).toBe("");
    expect(busyAgent(machine("/elsewhere", [pane("codex", "working")]), "/repo/app", "/repo/app", label)).toBe("");
    expect(busyAgent(machine("/var/t/app", [pane("codex", "working")]), "/private/var/t/app", "/var/t/app", label)).toBe("Codex"); // the folder as the lens names it, beside git's own spelling
  });
});
