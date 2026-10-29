import { describe, expect, it } from "vitest";
import type { Label, Status, Task } from "./api";
import { columnChoices, deleteWords, labelChoices, outcomeWords, plan, runInSequence, startPlan, startWords, type Request } from "./bulk";

const status = (id: number, title: string, project_id = 1): Status => ({ id, project_id, title, hex_color: "", kind: title === "Done" ? "done" : "open", position: id, on_new: false, on_start: false, on_finish: false, on_agent: false });
const label = (id: number, title: string): Label => ({ id, title, hex_color: "" });
const task = (id: number, project_id: number, status_id: number, extra: Partial<Task> = {}): Task => ({
  id,
  project_id,
  status_id,
  title: `t${id}`,
  description: "",
  done: false,
  done_at: "",
  due_date: "",
  priority: 0,
  position: 0,
  labels: [],
  fields: {},
  identifier: `#${id}`,
  source: "",
  created: "",
  updated: "",
  ...extra,
});
const p1 = [status(10, "New"), status(11, "In progress"), status(12, "Done")];
const p2 = [status(20, "new", 2), status(21, "Review", 2), status(22, "Done", 2)];
const columns = (t: Task) => (t.project_id === 1 ? p1 : p2);

describe("plan", () => {
  it("moves every task but the ones already there", () => {
    const { requests, skipped } = plan({ kind: "move", statusId: 11 }, [task(1, 1, 10), task(2, 1, 11), task(3, 1, 12)], columns);
    expect(requests).toEqual([
      { id: 1, op: "patch", fields: { status_id: 11 } },
      { id: 3, op: "patch", fields: { status_id: 11 } },
    ]);
    expect(skipped).toBe(1);
  });

  it("moves by column title in each task's own project, case-folded, skipping a project without it", () => {
    const { requests, skipped } = plan({ kind: "move-named", column: "new" }, [task(1, 1, 11), task(2, 2, 21), task(3, 2, 20)], columns);
    expect(requests.map((r) => r.id)).toEqual([1, 2]);
    expect(requests[1]).toEqual({ id: 2, op: "patch", fields: { status_id: 20 } });
    expect(skipped).toBe(1);
    expect(plan({ kind: "move-named", column: "Review" }, [task(1, 1, 10)], columns)).toEqual({ requests: [], skipped: 1 });
  });

  it("labels the tasks that lack the label, finishes the open ones, deletes all", () => {
    const bug = label(5, "bug");
    const ts = [task(1, 1, 10, { labels: [bug] }), task(2, 1, 10), task(3, 1, 12, { done: true })];
    expect(plan({ kind: "label", labelId: 5 }, ts, columns)).toEqual({ requests: [{ id: 2, op: "label", labelId: 5 }, { id: 3, op: "label", labelId: 5 }], skipped: 1 });
    expect(plan({ kind: "done" }, ts, columns)).toEqual({ requests: [{ id: 1, op: "patch", fields: { done: true } }, { id: 2, op: "patch", fields: { done: true } }], skipped: 1 });
    expect(plan({ kind: "delete" }, ts, columns).requests.map((r) => r.op)).toEqual(["delete", "delete", "delete"]);
  });
});

describe("choices", () => {
  it("offers one project's columns with their ids", () => {
    expect(columnChoices([task(1, 1, 10), task(2, 1, 11)], columns)).toEqual([
      { title: "New", statusId: 10, have: 2 },
      { title: "In progress", statusId: 11, have: 2 },
      { title: "Done", statusId: 12, have: 2 },
    ]);
  });

  it("offers the titles across projects by name, counting the tasks that can go there", () => {
    const choices = columnChoices([task(1, 1, 10), task(2, 2, 20), task(3, 2, 21)], columns);
    expect(choices).toEqual([
      { title: "New", have: 3 },
      { title: "In progress", have: 1 },
      { title: "Done", have: 3 },
      { title: "Review", have: 2 },
    ]);
    expect(columnChoices([], columns)).toEqual([]);
  });

  it("counts, per label, the tasks that lack it", () => {
    const bug = label(5, "bug");
    const choices = labelChoices([task(1, 1, 10, { labels: [bug] }), task(2, 1, 10)], [bug, label(6, "ui")]);
    expect(choices.map((c) => [c.label.title, c.lacking])).toEqual([
      ["bug", 1],
      ["ui", 2],
    ]);
  });
});

describe("runInSequence", () => {
  it("sends in order, goes on past a refusal and keeps the first error", async () => {
    const sent: number[] = [];
    const send = async (r: Request) => {
      sent.push(r.id);
      if (r.id === 2) throw new Error("no such column");
      if (r.id === 3) throw new Error("later");
    };
    const requests: Request[] = [1, 2, 3, 4].map((id) => ({ id, op: "delete" }));
    const outcome = await runInSequence(requests, send);
    expect(sent).toEqual([1, 2, 3, 4]);
    expect(outcome).toEqual({ done: 2, failed: 2, error: "no such column" });
    expect(outcomeWords({ kind: "delete" }, outcome, 0)).toBe("2 deleted, 2 failed: no such column");
    expect(outcomeWords({ kind: "move", statusId: 1 }, { done: 3, failed: 0, error: "" }, 1)).toBe("3 moved, 1 skipped");
    expect(outcomeWords({ kind: "label", labelId: 1 }, { done: 1, failed: 0, error: "" }, 0)).toBe("1 labelled");
    expect(deleteWords(2)).toBe("Delete 2 tasks? They are gone for good.");
  });
});

describe("startPlan", () => {
  const running = (id: number, state: "starting" | "running" | "finished" | "gone" | "failed") =>
    task(id, 1, 10, { run: { id, task_id: id, machine: "local", workspace: "w1", pane: "w1:p1", dir: "/x", branch: "b", agent: "claude", state, started: "" } as Task["run"] });
  it("skips the tasks an agent is still on and names them", () => {
    const { starts, skipped } = startPlan([task(1, 1, 10), running(2, "running"), running(3, "finished"), running(4, "starting")]);
    expect(starts.map((t) => t.id)).toEqual([1, 3]);
    expect(skipped.map((t) => t.id)).toEqual([2, 4]);
    expect(startWords({ done: 2, failed: 0, error: "" }, skipped)).toBe("Started 2 agents, 2 skipped: #2, #4 already run");
    expect(startWords({ done: 1, failed: 1, error: "no such branch" }, [])).toBe("Started 1 agent, 1 refused: no such branch");
  });
});
