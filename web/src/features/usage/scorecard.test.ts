import { describe, expect, it } from "vitest";
import type { Mean, ScoreRow, ScoreTry, Share } from "./api";
import { bestByColumn, bestOf, costWords, fewWords, figureOf, finishedWords, minutesWords, modelName, periodOf, reviewWords, rowKey, rowName, shareWords, tryLanes, tryWords } from "./scorecard";

const share = (count: number, of: number, rate?: number): Share => ({ count, of, rate });
const mean = (value: number | undefined, n: number): Mean => ({ value, n, few: n < 5 });
const row = (over: Partial<ScoreRow> = {}): ScoreRow => ({
  agent: "claude",
  model: "Opus 4.5",
  model_id: "claude-opus-4-5",
  runs: 6,
  finished: share(6, 6, 1),
  first_approved: share(3, 5, 0.6),
  rejected: 2,
  approved: 4,
  merged: share(1, 2),
  kept: share(0, 0),
  cost_per_run: mean(2.5, 6),
  cost_per_approved: mean(4, 4),
  minutes_per_run: mean(12.4, 6),
  last: "2026-11-10T09:00:00Z",
  ...over,
});

describe("shareWords", () => {
  it("says a percentage only once enough runs stand behind it", () => {
    expect(shareWords(share(3, 5, 0.6))).toBe("60 %");
    expect(shareWords(share(2, 3))).toBe("2 of 3");
    expect(shareWords(share(0, 0))).toBe("—");
  });
});

describe("the means", () => {
  it("are dollars and minutes, a dash with nothing measured", () => {
    expect(costWords(mean(2.5, 6))).toBe("$2.50");
    expect(costWords(mean(undefined, 0))).toBe("—");
    expect(minutesWords(mean(4.25, 6))).toBe("4.3 min");
    expect(minutesWords(mean(38.6, 6))).toBe("39 min");
    expect(minutesWords(mean(undefined, 0))).toBe("—");
  });

  it("say when they stand on too few runs", () => {
    expect(fewWords(2, 5)).toBe("from 2 runs only");
    expect(fewWords(1, 5)).toBe("from 1 run only");
    expect(fewWords(5, 5)).toBe("");
    expect(fewWords(0, 5)).toBe("");
  });
});

describe("bestOf", () => {
  const fable = row({ model: "Fable 5.1", first_approved: share(4, 5, 0.8), cost_per_run: mean(3, 6), minutes_per_run: mean(9, 6) });
  const gpt = row({ agent: "codex", model: "gpt-6.1-sol", first_approved: share(2, 3), cost_per_run: mean(undefined, 0), minutes_per_run: mean(5, 2) });
  const rows = [row(), fable, gpt];

  it("marks the highest share and the lowest cost or time among the rows with enough runs", () => {
    expect([...bestOf(rows, "first_approved")]).toEqual([rowKey(fable)]);
    expect([...bestOf(rows, "cost_per_run")]).toEqual([rowKey(row())]);
    expect([...bestOf(rows, "minutes_per_run")]).toEqual([rowKey(fable)]); // gpt's five minutes stand on two runs
  });

  it("marks nothing with fewer than two rows to compare, and every row of a tie", () => {
    expect(bestOf(rows, "merged").size).toBe(0);
    expect(bestOf([row()], "finished").size).toBe(0);
    expect(bestOf([row(), fable], "finished")).toEqual(new Set([rowKey(row()), rowKey(fable)]));
  });

  it("is read for every column at once", () => {
    const all = bestByColumn(rows);
    expect(Object.keys(all).sort()).toEqual(["cost_per_approved", "cost_per_run", "finished", "first_approved", "kept", "merged", "minutes_per_run"]);
    expect([...all.first_approved]).toEqual([rowKey(fable)]);
  });

  it("compares a mean only once it is not few", () => {
    expect(figureOf(gpt, "minutes_per_run")).toBeUndefined();
    expect(figureOf(row(), "cost_per_run")).toBe(2.5);
    expect(figureOf(gpt, "first_approved")).toBeUndefined();
  });
});

describe("a row's words", () => {
  it("names the agent and the model, or that no model was read", () => {
    expect(rowName(row())).toBe("Claude Code · Opus 4.5");
    expect(rowName(row({ agent: "codex", model: "" }))).toBe("Codex · model not read");
    expect(modelName({ model: "" })).toBe("model not read");
  });

  it("says what the reviews came to", () => {
    expect(reviewWords(row())).toBe("3 approved at the first review, 2 rejected before an approval, 1 of them approved later");
    expect(reviewWords(row({ first_approved: share(0, 0), rejected: 0, approved: 0 }))).toBe("No run of this model was reviewed in the Changes lens yet");
  });

  it("says the runs still at work are left out of the finished share", () => {
    expect(finishedWords(row(), 5)).toBe("");
    expect(finishedWords(row({ finished: share(2, 3), under_way: 1 }), 5)).toBe("from 3 runs only · 1 run still at work, not counted yet");
    expect(finishedWords(row({ under_way: 2 }), 5)).toBe("2 runs still at work, not counted yet");
  });

  it("keeps a stored period to the ones offered", () => {
    expect(periodOf(7)).toBe(7);
    expect(periodOf(14)).toBe(30);
  });
});

describe("trying a model on a past task", () => {
  const pick: ScoreTry = {
    task_id: 12,
    identifier: "#12",
    title: "Fix the login page",
    project_id: 1,
    machine: "studio",
    dir: "/home/me/.herdr/worktrees/app/task-12",
    ran: "2026-11-09T10:00:00Z",
    lanes: [
      { agent: "claude", model: "Fable 5.1", model_id: "claude-fable-5-1" },
      { agent: "claude", model: "Opus 4.5", model_id: "claude-opus-4-5" },
    ],
  };

  it("hands the arena the ids the agents' --model takes", () => {
    expect(tryLanes(pick)).toEqual([
      { agent: "claude", model: "claude-fable-5-1" },
      { agent: "claude", model: "claude-opus-4-5" },
    ]);
    expect(tryLanes({ ...pick, lanes: [{ agent: "codex" }] })).toEqual([{ agent: "codex", model: "" }]);
  });

  it("says which task and which two models", () => {
    expect(tryWords(pick)).toBe("Try it on a past task: #12 Fix the login page in an arena, Claude Code · Fable 5.1 beside Claude Code · Opus 4.5");
    expect(tryWords({ ...pick, lanes: pick.lanes.slice(0, 1) })).toBe("Try it on a past task: #12 Fix the login page in an arena");
  });
});
