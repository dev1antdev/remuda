import { describe, expect, it } from "vitest";
import type { Machine, Session } from "@/features/roster/api";
import type { Recipe } from "@/features/recipes/api";
import type { Action, Layout, Run, Scene, Step } from "./api";
import {
  actionHint,
  actionWords,
  alreadyRuns,
  firstPicks,
  itemWords,
  layoutWords,
  moveStep,
  newTidyScene,
  runWords,
  sceneWords,
  sourceOf,
  stepsOfWorkspaces,
  stepWhat,
  stepWhere,
  stepWords,
  tidyWords,
  whyNotSave,
  withSource,
  worktreeName,
} from "./scenes";

const agents = [
  { id: "claude", label: "Claude Code" },
  { id: "codex", label: "Codex" },
];
const recipes: Recipe[] = [{ id: "r1", name: "Planner", agent: "claude", model: "opus" }];

const session = (dir: string, agent: string, scratch = false): Session => ({
  id: dir,
  name: dir.split("/").pop() ?? "",
  status: "idle",
  active_tab_id: "t1",
  dir,
  scratch,
  tabs: [{ id: "t1", label: "1", number: 1, named: false, status: "idle", panes: [{ id: "p1", title: "", agent, cwd: dir, focused: false, status: "idle", cols: 80, rows: 24 }] }],
});
const machine = (id: string, state: Machine["state"], sessions: Session[] = []): Machine => ({ id, label: id.toUpperCase(), state, sessions });
const here = machine("local", "connected", [session("/home/me/api", "claude"), session("/home/me/notes", ""), session("/home/me", "", true)]);
const machines = [here, machine("gog", "error")];

describe("a step", () => {
  it("runs an agent, a recipe or a shell, as the source Select picks", () => {
    const step: Step = { machine: "local", dir: "/a", agent: "claude", model: "opus", args: "--x", prompt: "Plan the day", tab: "plan" };
    expect(sourceOf(step)).toBe("agent:claude");
    expect(withSource(step, "recipe:r1")).toEqual({ machine: "local", dir: "/a", tab: "plan", recipe: "r1" });
    expect(withSource(step, "shell")).toEqual({ machine: "local", dir: "/a", tab: "plan", agent: "shell" });
    expect(withSource(step, "agent:codex")).toEqual({ machine: "local", dir: "/a", tab: "plan", agent: "codex", prompt: "Plan the day" });
    expect(sourceOf({ machine: "local", dir: "/a", recipe: "r1" })).toBe("recipe:r1");
    expect(sourceOf({ machine: "local", dir: "/a" })).toBe("shell");
  });

  it("says what it runs and where", () => {
    expect(stepWhat({ machine: "local", dir: "/a", agent: "claude", model: "opus", prompt: "Read the overnight pull requests and sum them up" }, agents, recipes)).toBe("Claude Code · Opus · Read the overnight pull requests and sum…");
    expect(stepWhat({ machine: "local", dir: "/a", recipe: "r1" }, agents, recipes)).toBe("recipe Planner");
    expect(stepWhat({ machine: "local", dir: "/a", recipe: "gone" }, agents, recipes)).toBe("a recipe that is gone");
    expect(stepWhat({ machine: "local", dir: "/a", agent: "shell" }, agents, recipes)).toBe("Plain shell");
    expect(stepWhere({ machine: "gog", dir: "/home/me/web" }, machines)).toBe("web on GOG");
    expect(stepWhere({ machine: "local", dir: "/home/me/web" }, [here])).toBe("web");
    expect(stepWords({ machine: "local", dir: "/a", agent: "shell", tab: "notes" }, agents, recipes)).toBe("Plain shell · tab “notes”");
  });

  it("says what its layout lays out", () => {
    const layout: Layout = { agent: 1, root: { type: "split", direction: "right", ratio: 0.5, first: { type: "pane", command: ["npm", "run", "dev"] }, second: { type: "pane" } } };
    expect(layoutWords(layout)).toBe("2 panes · the agent in pane 2 · runs npm run dev");
    expect(layoutWords({ agent: 0, root: { type: "pane" } })).toBe("1 pane");
    expect(layoutWords(layout, false)).toBe("2 panes · runs npm run dev");
    expect(stepWords({ machine: "local", dir: "/a", agent: "shell", layout }, agents, recipes)).toBe("Plain shell · 2 panes · runs npm run dev");
  });

  it("moves up and down, never past either end", () => {
    const steps: Step[] = [{ machine: "a", dir: "/1" }, { machine: "a", dir: "/2" }];
    expect(moveStep(steps, 1, -1).map((s) => s.dir)).toEqual(["/2", "/1"]);
    expect(moveStep(steps, 0, -1)).toBe(steps);
    expect(moveStep(steps, 1, 1)).toBe(steps);
  });
});

describe("a run of a start scene", () => {
  it("starts ticked only what is not away and not already running", () => {
    const steps: Step[] = [
      { machine: "local", dir: "/home/me/api", agent: "claude" }, // Claude Code runs there already
      { machine: "local", dir: "/home/me/api", agent: "codex" },
      { machine: "gog", dir: "/home/me/web", agent: "claude" }, // away
      { machine: "local", dir: "/home/me/api", recipe: "r1" }, // the recipe's Claude Code runs there already
      { machine: "local", dir: "/home/me/notes", agent: "shell" }, // a shell never counts as running
    ];
    expect(alreadyRuns(steps[0]!, machines, recipes)).toBe(true);
    expect(alreadyRuns(steps[3]!, machines, recipes)).toBe(true);
    expect(firstPicks(steps, machines, recipes)).toEqual([1, 4]);
  });

  it("is built from the workspaces open now, the scratch shell and the machines away left out", () => {
    expect(stepsOfWorkspaces(machines)).toEqual([
      { machine: "local", dir: "/home/me/api", agent: "claude" },
      { machine: "local", dir: "/home/me/notes", agent: "shell" },
    ]);
  });
});

describe("a tidy", () => {
  it("says what it does and where", () => {
    const tidy = { shells: true, idle: true, idle_hours: 8, compact: true, compact_at: 70, worktrees: false, machines: ["gog"] };
    expect(tidyWords(tidy, machines)).toBe("/compact idle Claude Codes past 70 % of their context · Close agents idle 8 hours or more · Close the shells sitting at their prompt · GOG");
    expect(tidyWords(newTidyScene().tidy!, machines)).toContain("every machine");
  });

  const now = Date.UTC(2026, 10, 12, 21);
  it("names each action, and what goes with it", () => {
    const close: Action = { id: "x", kind: "close-agent", machine: "local", pane: "w1:p1", workspace: "api", tab: "planner", agent: "claude", since: now - 10 * 3_600_000, closes: "tab" };
    expect(actionWords(close, agents, now)).toBe("Close Claude Code in api · planner, idle 10 h");
    expect(actionHint(close)).toBe("its session stays: Reopen a closed agent · its tab closes with it");
    const shell: Action = { id: "y", kind: "close-shell", machine: "local", pane: "w2:p1", workspace: "web", tab: "1", closes: "workspace" };
    expect(actionWords(shell, agents, now)).toBe("Close the shell in web");
    expect(actionHint(shell)).toBe("its workspace closes with it");
    expect(actionWords({ id: "z", kind: "compact", machine: "local", pane: "w1:p2", workspace: "api", agent: "claude", context: 85 }, agents, now)).toBe("/compact Claude Code in api, at 85 % of its context");
    const tree: Action = { id: "w", kind: "worktree", machine: "local", path: "/home/me/.herdr/worktrees/api/old", branch: "old", kb: 2048 };
    expect(actionWords(tree, agents, now)).toBe("Remove the worktree api/old");
    expect(actionHint(tree)).toBe("old · 2.0 MB");
    expect(worktreeName("/x/y/")).toBe("x/y");
  });
});

describe("the outcome", () => {
  it("says how each item went, and counts them", () => {
    expect(itemWords({ index: 0, state: "started", made_workspace: true })).toBe("started in a new workspace");
    expect(itemWords({ index: 0, state: "started" })).toBe("started in a new tab");
    expect(itemWords({ index: 0, state: "away", note: "not connected" })).toBe("its machine is away: not connected");
    expect(itemWords({ index: 0, state: "skipped", note: "it works now" })).toBe("skipped: it works now");
    const run: Run = { id: "r", scene: "s", name: "Morning", kind: "start", at: "", done: true, items: [{ index: 0, state: "started" }, { index: 1, state: "started" }, { index: 2, state: "away" }] };
    expect(runWords(run)).toBe("2 started, 1 away");
    expect(runWords({ ...run, items: [] })).toBe("nothing to do");
  });
});

describe("a scene", () => {
  it("is saved only once it can run", () => {
    const morning: Scene = { name: "Morning", kind: "start", steps: [{ machine: "local", dir: "/home/me/api", agent: "claude" }] };
    expect(whyNotSave(morning)).toBe("");
    expect(whyNotSave({ ...morning, name: " " })).toBe("A scene needs a name.");
    expect(whyNotSave({ ...morning, steps: [] })).toBe("Add a step.");
    expect(whyNotSave({ ...morning, steps: [...morning.steps!, { machine: "local", dir: "" }] })).toBe("Step 2 needs a folder.");
    expect(whyNotSave({ name: "Evening", kind: "tidy", tidy: { shells: false, idle: false, compact: false, worktrees: false } })).toBe("Pick at least one thing to tidy.");
    expect(whyNotSave(newTidyScene())).toBe("");
  });

  it("says what it holds in the list", () => {
    const morning: Scene = { name: "Morning", kind: "start", steps: [{ machine: "local", dir: "/home/me/api", agent: "claude" }] };
    expect(sceneWords(morning, agents, recipes, [here])).toBe("1 step · api: Claude Code");
  });
});
