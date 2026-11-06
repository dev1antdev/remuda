import { describe, expect, it } from "vitest";
import type { Machine } from "@/features/roster/api";
import type { Project, Run, Status } from "./api";
import { automationOf, queueWords, repositories, saveable, sameAutomation, stageWords, whyNotQueue, whyNotSave, NO_AUTOMATION } from "./autonomy";

const col = (id: number, more: Partial<Status> = {}): Status => ({ id, project_id: 1, title: `c${id}`, hex_color: "", kind: "open", position: id, on_new: false, on_start: false, on_finish: false, on_agent: false, ...more });
const project = (more: Partial<Project> = {}): Project => ({
  id: 1,
  title: "App",
  description: "",
  hex_color: "",
  parent_project_id: 0,
  is_archived: false,
  is_favorite: false,
  position: 1,
  created: "",
  updated: "",
  statuses: [col(1, { on_new: true }), col(2, { on_queue: true }), col(3, { kind: "done" })],
  fields: [],
  proposed: 0,
  ...more,
});
const run = (more: Partial<Run>): Run => ({ id: 1, task_id: 1, machine: "local", workspace: "w1", pane: "w1:p1", dir: "/w", branch: "b", agent: "claude", state: "running", started: "", ...more });

describe("queueWords", () => {
  it("says the queue is off, looking, idle, waiting and why, or what it just started", () => {
    expect(queueWords(project()).tone).toBe("dim");
    expect(queueWords(project()).text).toMatch(/switch the queue on/);
    const on = { ...NO_AUTOMATION, machine: "gog", dir: "/w/app", queue: true, max: 2 };
    expect(queueWords(project({ automation: on })).text).toBe("The queue looks here every half minute");
    expect(queueWords(project({ automation: on, queue: { state: "idle", working: 1, max: 2, at: "" } })).text).toBe("Starts the next card put here · 1 of 2 working");
    expect(queueWords(project({ automation: on, queue: { state: "started", next: "#12", working: 2, max: 2, at: "" } }))).toEqual({ text: "Just started #12 · 2 of 2 working", tone: "green" });
    expect(queueWords(project({ automation: on, queue: { state: "waiting", next: "#13", reason: "GOG is away", working: 0, max: 2, at: "" } }))).toEqual({ text: "#13 waits: GOG is away", tone: "amber" });
    expect(queueWords(project({ automation: on, queue: { state: "waiting", next: "#13", reason: "every slot is taken: 2 of 2 working", working: 2, max: 2, at: "" } })).tone).toBe("dim");
    expect(queueWords(project({ automation: on, queue: { state: "waiting", reason: "no column is marked Queued", working: 0, max: 2, at: "" } })).text).toBe("Waits: no column is marked Queued");
  });
});

describe("stageWords", () => {
  const piped = project({ automation: { ...NO_AUTOMATION, stages: [{ name: "Tests", agent: "codex", prompt: "x" }, { name: "PR", agent: "claude", prompt: "", pr: true }] } });
  it("says where a run stands in its pipeline, nothing outside one", () => {
    expect(stageWords(run({}), piped)).toBe("");
    expect(stageWords(run({ pipeline: "on", stage: 0 }), piped)).toBe("Pipeline: the first agent's work, then 2 stages");
    expect(stageWords(run({ pipeline: "on", stage: 2 }), piped)).toBe("Pipeline stage 2 of 2 · PR");
    expect(stageWords(run({ pipeline: "passed", stage: 1 }), piped)).toBe("Pipeline stage 1 of 2 · Tests");
    expect(stageWords(run({ pipeline: "done", stage: 2 }), piped)).toBe("Pipeline done");
    expect(stageWords(run({ pipeline: "stopped: the agent asked something", stage: 1 }), piped)).toBe("Pipeline stopped: the agent asked something");
  });
});

describe("the settings", () => {
  it("fills the defaults in for a project from before them", () => {
    expect(automationOf(project())).toEqual(NO_AUTOMATION);
    expect(automationOf(project({ automation: { ...NO_AUTOMATION, queue: true, stages: undefined as never } })).stages).toEqual([]);
  });
  it("lets go of a stage's column the edit removed, and compares what the settings say", () => {
    const a = { ...NO_AUTOMATION, stages: [{ name: "T", agent: "codex", prompt: "x", status_id: 9 }, { name: "R", agent: "claude", prompt: "y", status_id: 2 }] };
    expect(saveable(a, [col(1), col(2), col(0)]).stages.map((s) => s.status_id)).toEqual([0, 2]);
    expect(sameAutomation(a, { ...a, model: "" })).toBe(true);
    expect(sameAutomation(a, { ...a, max: 2 })).toBe(false);
  });
  it("says what the queue's switch still needs", () => {
    expect(whyNotQueue(NO_AUTOMATION, project().statuses)).toMatch(/where agents work/);
    expect(whyNotQueue({ ...NO_AUTOMATION, machine: "gog", dir: "/w" }, [col(1)])).toMatch(/Queued/);
    expect(whyNotQueue({ ...NO_AUTOMATION, machine: "gog", dir: "/w" }, project().statuses)).toBe("");
  });
  it("keeps the save from what the server would refuse after the columns are saved", () => {
    const at = { ...NO_AUTOMATION, machine: "gog", dir: "/w" };
    expect(whyNotSave({ ...at, queue: true }, project().statuses)).toBe("");
    expect(whyNotSave({ ...at, queue: true }, [col(1), col(3, { kind: "done" })])).toMatch(/column marked Queued/);
    expect(whyNotSave({ ...NO_AUTOMATION, queue: true }, project().statuses)).toMatch(/machine and a repository/);
    expect(whyNotSave(at, [col(1)])).toBe("");
    expect(whyNotSave({ ...at, stages: [{ name: "T", agent: "codex", prompt: "x" }, { name: "R", agent: "claude", prompt: "  " }] }, [col(1)])).toBe("Stage 2 needs a prompt");
    expect(whyNotSave({ ...at, stages: [{ name: "PR", agent: "claude", prompt: "", pr: true }] }, [col(1)])).toBe("");
  });
  it("offers a machine's repositories once each, never a linked worktree", () => {
    const machines = [
      {
        id: "gog",
        sessions: [
          { dir: "/w/app", worktree: { linked: false } },
          { dir: "/w/app" },
          { dir: "/h/.herdr/worktrees/app/x", worktree: { linked: true } },
          { dir: "" },
        ],
      },
    ] as unknown as Machine[];
    expect(repositories(machines, "gog")).toEqual([{ value: "/w/app", label: "app", hint: "/w/app" }]);
    expect(repositories(machines, "nope")).toEqual([]);
  });
});
