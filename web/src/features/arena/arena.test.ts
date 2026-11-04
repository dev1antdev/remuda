import { describe, expect, it } from "vitest";
import type { AgentKind, Machine, Pane, Session } from "@/features/roster/api";
import { CUSTOM } from "@/features/roster/startOptions";
import type { Lane } from "./api";
import { defaultLanes, keepWords, modelOf, paneOf, repositories, requestOf, runSeconds, statWords, whyNot, type LaneDraft } from "./arena";

const pane = (id: string, over: Partial<Pane> = {}): Pane => ({ id, title: "", agent: "claude", cwd: "/x", focused: false, status: "idle", cols: 80, rows: 24, ...over }) as Pane;
const session = (id: string, over: Partial<Session> = {}): Session => ({ id, name: id, status: "idle", active_tab_id: "", tabs: [], ...over }) as Session;
const machine = (id: string, sessions: Session[], state = "connected"): Machine => ({ id, label: id, state, sessions }) as unknown as Machine;
const agents: AgentKind[] = [{ id: "claude", label: "Claude Code" }, { id: "codex", label: "Codex" }, { id: "shell", label: "Shell" }];

describe("repositories", () => {
  it("lists a machine's repositories, not their worktrees", () => {
    const m = machine("local", [session("w1", { dir: "/p/api" }), session("w2", { dir: "/w/api-x", worktree: { repo_key: "/p/api/.git", repo_name: "api", linked: true, checkout_path: "/w/api-x" } }), session("w3")]);
    expect(repositories([m], "local").map((r) => r.value)).toEqual(["w1"]);
  });
});

describe("defaultLanes", () => {
  it("puts two different agents on the repository on screen", () => {
    const machines = [machine("local", [session("w1", { dir: "/p/a" }), session("w2", { dir: "/p/b" })]), machine("gog", [session("w9", { dir: "/p/c" })])];
    expect(defaultLanes(machines, agents, { machineId: "local", workspaceId: "w2" })).toEqual([
      { machine: "local", workspace: "w2", agent: "claude", choice: "", custom: "" },
      { machine: "local", workspace: "w2", agent: "codex", choice: "", custom: "" },
    ]);
  });

  it("falls back to the first connected machine with a repository, and one agent twice when only one is known", () => {
    const machines = [machine("away", [session("w1", { dir: "/p/a" })], "error"), machine("gog", [session("w9", { dir: "/p/c" })])];
    const lanes = defaultLanes(machines, [{ id: "claude", label: "Claude Code" }]);
    expect(lanes.map((l) => `${l.machine}/${l.workspace}/${l.agent}`)).toEqual(["gog/w9/claude", "gog/w9/claude"]);
  });
});

describe("whyNot", () => {
  const lane = (over: Partial<LaneDraft> = {}): LaneDraft => ({ machine: "local", workspace: "w1", agent: "claude", choice: "", custom: "", ...over });
  it("says what is missing before an arena can start", () => {
    expect(whyNot("", [lane(), lane()])).toBe("Write the prompt every agent gets");
    expect(whyNot("fix it", [lane()])).toBe("An arena needs at least two agents");
    expect(whyNot("fix it", Array.from({ length: 7 }, () => lane()))).toBe("An arena has at most six agents");
    expect(whyNot("fix it", [lane(), lane({ workspace: "" })])).toBe("Lane 2 needs a machine and a repository");
    expect(whyNot("fix it", [lane(), lane({ choice: CUSTOM })])).toBe("Type the model's id, or pick the default");
    expect(whyNot("fix it", [lane(), lane({ agent: "codex" })])).toBe("");
  });

  it("sends the typed model behind the custom choice", () => {
    expect(modelOf(lane({ choice: "opus" }))).toBe("opus");
    expect(modelOf(lane({ choice: CUSTOM, custom: " gpt-6 " }))).toBe("gpt-6");
    expect(requestOf([lane({ choice: "sonnet" })])).toEqual([{ machine: "local", workspace: "w1", agent: "claude", model: "sonnet" }]);
  });
});

describe("a lane's card", () => {
  const now = Date.parse("2026-11-04T12:00:00Z");
  const lane = (over: Partial<Lane> = {}): Lane => ({ id: 1, machine: "local", workspace: "w1", agent: "claude", state: "running", pane: "w5:p1", started: "2026-11-04T11:50:00Z", ...over });

  it("says the work at a glance", () => {
    expect(statWords(undefined)).toBe("");
    expect(statWords({ added: 0, removed: 0, files: 0 })).toBe("no changes yet");
    expect(statWords({ added: 12, removed: 3, files: 4 })).toBe("+12 −3 · 4 files");
    expect(statWords({ added: 1, removed: 0, files: 1 })).toBe("+1 −0 · 1 file");
  });

  it("finds its pane and counts its time to now while it works, to its last turn once it rests", () => {
    const machines = [machine("local", [session("w5", { tabs: [{ id: "t1", panes: [pane("w5:p1", { radar: { state: "working" } })] }] as unknown as Session["tabs"] })])];
    const p = paneOf(machines, lane());
    expect(p?.id).toBe("w5:p1");
    expect(runSeconds(lane(), p, now)).toBe(600);
    const resting = pane("w5:p1", { radar: { state: "done", last_active: Date.parse("2026-11-04T11:55:00Z") } });
    expect(runSeconds(lane(), resting, now)).toBe(300);
    expect(runSeconds(lane({ started: undefined }), resting, now)).toBe(0);
    expect(runSeconds(lane({ state: "discarded" }), undefined, now)).toBe(0);
    expect(runSeconds(lane({ state: "starting" }), undefined, now)).toBe(600);
    expect(paneOf(machines, lane({ pane: "w5:p9" }))).toBeUndefined();
  });

  it("asks before keeping, saying what happens to the rest", () => {
    const lanes = [lane({ id: 1 }), lane({ id: 2, dir: "/w/2" }), lane({ id: 3, dir: "/w/3" }), lane({ id: 4, state: "discarded", dir: "/w/4" })];
    expect(keepWords(lanes, 1)).toContain("close the other 2");
    expect(keepWords([lane({ id: 1 }), lane({ id: 2, dir: "/w/2" })], 1)).toContain("close the other one");
    expect(keepWords([lane({ id: 1 })], 1)).toBe("Keep this lane? No other lane is open.");
  });
});
