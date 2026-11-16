import { describe, expect, it } from "vitest";
import { syncRows } from "./syncMenu";

const labels = (rows: ReturnType<typeof syncRows>) => rows.map((r) => ("label" in r ? `${r.label}${r.disabled ? " (off)" : ""}` : "—"));

describe("the Git lens's sync menu", () => {
  it("offers fetch, pull and push on a branch", () => {
    expect(labels(syncRows("main", "", () => {}))).toEqual(["Fetch", "Pull", "Push", "Push without hooks"]);
  });

  it("offers the fetch alone while HEAD is detached", () => {
    expect(labels(syncRows("", "", () => {}))).toEqual(["Fetch", "Pull (off)", "Push (off)", "Push without hooks (off)"]);
  });

  it("offers nothing while one runs", () => {
    expect(labels(syncRows("main", "push", () => {}))).toEqual(["Fetch (off)", "Pull (off)", "Push (off)", "Push without hooks (off)"]);
  });

  it("runs the action picked", () => {
    const ran: string[] = [];
    const [, pull, push, pushWithoutHooks] = syncRows("main", "", (action, noVerify) => ran.push(noVerify ? `${action} --no-verify` : action));
    for (const row of [pull, push, pushWithoutHooks]) if (row && "onSelect" in row) row.onSelect();
    expect(ran).toEqual(["pull", "push", "push --no-verify"]);
  });
});
