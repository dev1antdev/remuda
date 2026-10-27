// The agent's plan as the page reads it: how far along it is, what it is on, the "3/7" for a tile's state line, and
// the one line a TodoWrite / TaskCreate / TaskUpdate call makes in the turn view. Pure; tested beside.

import type { Part, PlanItem } from "./api";
import { t, tn } from "@/lib/i18n";

export const PLAN_TOOLS = new Set(["TodoWrite", "TaskCreate", "TaskUpdate"]);

export type PlanCounts = { done: number; total: number; open: number };

export function counts(plan: PlanItem[]): PlanCounts {
  const done = plan.filter((item) => item.status === "completed").length;
  return { done, total: plan.length, open: plan.length - done };
}

// current is the item the agent is on: the last one in progress, else none
export function current(plan: PlanItem[]): PlanItem | null {
  for (let i = plan.length - 1; i >= 0; i--) if (plan[i]!.status === "in_progress") return plan[i]!;
  return null;
}

// currentWords is how the agent says what it is on: its active form, else the item itself
export const currentWords = (item: PlanItem): string => item.active || item.text;

// progressWords is the tile's "3/7"; "" without a plan
export function progressWords(plan: PlanItem[]): string {
  const { done, total } = counts(plan);
  return total ? `${done}/${total}` : "";
}

// planLine is the bar's one line: how far, and what the agent is on, or that it is all done
export function planLine(plan: PlanItem[]): string {
  const words = progressWords(plan);
  if (!words) return "";
  const now = current(plan);
  if (now) return `${words} · ${currentWords(now)}`;
  return counts(plan).open ? words : `${words} · ${t("done")}`;
}

// taskNumber is the id Claude Code gave a created task, read off the tool's answer ("Task #3 created…")
export function taskNumber(output: string | undefined): string {
  const m = /^Task #(\d+) created/.exec(output ?? "");
  return m ? m[1]! : "";
}

export type PlanCall = { words: string; mark: PlanItem["status"] | "list" };

// planCallWords is the one line a plan tool's call makes in a turn: what the agent set out to do or moved
export function planCallWords(part: Part): PlanCall {
  const input = (typeof part.input === "object" && part.input ? part.input : {}) as Record<string, unknown>;
  const str = (key: string): string => (typeof input[key] === "string" ? (input[key] as string) : "");
  switch (part.tool_name) {
    case "TodoWrite": {
      const todos = Array.isArray(input["todos"]) ? (input["todos"] as { status?: string }[]) : [];
      if (!todos.length) return { words: "Plan", mark: "list" };
      const done = todos.filter((todo) => todo.status === "completed").length;
      return { words: tn(todos.length, "Plan: {n} item, {done} done", "Plan: {n} items, {done} done", { done }), mark: "list" };
    }
    case "TaskCreate": {
      const id = taskNumber(part.output);
      return { words: `Task${id ? ` #${id}` : ""}: ${str("subject")}`, mark: "pending" };
    }
    case "TaskUpdate": {
      const task = t("Task #{id}", { id: str("taskId") });
      const status = str("status");
      if (status === "completed" || status === "in_progress") return { words: `${task} → ${status.replace("_", " ")}`, mark: status };
      if (status === "deleted") return { words: t("{task} deleted", { task }), mark: "pending" };
      return { words: status ? `${task} → ${status}` : t("{task} edited", { task }), mark: "pending" };
    }
  }
  return { words: part.tool_name ?? "", mark: "list" };
}
