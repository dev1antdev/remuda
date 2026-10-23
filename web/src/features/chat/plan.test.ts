import { describe, expect, it } from "vitest";
import type { Part, PlanItem } from "./api";
import { counts, current, planCallWords, planLine, progressWords, taskNumber } from "./plan";

const item = (id: string, status: PlanItem["status"], text = `task ${id}`, active?: string): PlanItem => ({ id, text, status, active });

describe("plan", () => {
  it("counts what is done and what is left", () => {
    expect(counts([item("1", "completed"), item("2", "in_progress"), item("3", "pending")])).toEqual({ done: 1, total: 3, open: 2 });
    expect(counts([])).toEqual({ done: 0, total: 0, open: 0 });
    expect(progressWords([item("1", "completed"), item("2", "pending")])).toBe("1/2");
    expect(progressWords([])).toBe("");
  });

  it("picks the last item in progress as the one the agent is on", () => {
    expect(current([item("1", "in_progress"), item("2", "in_progress"), item("3", "pending")])?.id).toBe("2");
    expect(current([item("1", "completed"), item("2", "pending")])).toBeNull();
  });

  it("says how far along and what the agent is on, in its own words", () => {
    expect(planLine([item("1", "completed"), item("2", "in_progress", "Build the page", "Building the page"), item("3", "pending")])).toBe("1/3 · Building the page");
    expect(planLine([item("1", "completed"), item("2", "in_progress", "Build the page")])).toBe("1/2 · Build the page");
    expect(planLine([item("1", "completed"), item("2", "pending")])).toBe("1/2");
    expect(planLine([item("1", "completed")])).toBe("1/1 · done");
    expect(planLine([])).toBe("");
  });

  it("makes one line of a plan tool's call", () => {
    const create: Part = { type: "tool", tool_name: "TaskCreate", input: { subject: "Scaffold the server" }, output: "Task #1 created successfully: Scaffold the server" };
    expect(planCallWords(create)).toEqual({ words: "Task #1: Scaffold the server", mark: "pending" });
    expect(planCallWords({ type: "tool", tool_name: "TaskCreate", input: { subject: "Ship it" } })).toEqual({ words: "Task: Ship it", mark: "pending" });
    expect(planCallWords({ type: "tool", tool_name: "TaskUpdate", input: { taskId: "1", status: "completed" } })).toEqual({ words: "Task #1 → completed", mark: "completed" });
    expect(planCallWords({ type: "tool", tool_name: "TaskUpdate", input: { taskId: "2", status: "in_progress" } })).toEqual({ words: "Task #2 → in progress", mark: "in_progress" });
    expect(planCallWords({ type: "tool", tool_name: "TaskUpdate", input: { taskId: "2", status: "deleted" } })).toEqual({ words: "Task #2 deleted", mark: "pending" });
    expect(planCallWords({ type: "tool", tool_name: "TaskUpdate", input: { taskId: "2", subject: "New name" } })).toEqual({ words: "Task #2 edited", mark: "pending" });
    expect(planCallWords({ type: "tool", tool_name: "TodoWrite", input: { todos: [{ content: "a", status: "completed" }, { content: "b", status: "pending" }] } })).toEqual({ words: "Plan: 2 items, 1 done", mark: "list" });
    expect(planCallWords({ type: "tool", tool_name: "TodoWrite", input: "a list the server cut" })).toEqual({ words: "Plan", mark: "list" });
  });

  it("reads the task number off the tool's answer", () => {
    expect(taskNumber("Task #12 created successfully: x")).toBe("12");
    expect(taskNumber("Updated task #1 status")).toBe("");
    expect(taskNumber(undefined)).toBe("");
  });
});
