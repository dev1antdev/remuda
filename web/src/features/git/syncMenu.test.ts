import { describe, expect, it } from "vitest";
import { syncRows } from "./syncMenu";

const labels = (rows: ReturnType<typeof syncRows>) => rows.map((r) => ("label" in r ? `${r.label}${r.disabled ? " (off)" : ""}` : "—"));

describe("the Git lens's sync menu", () => {
  it("offers fetch, pull and push on a branch", () => {
    expect(labels(syncRows("main", "", () => {}))).toEqual(["Fetch", "Pull", "Push"]);
  });

  it("offers the fetch alone while HEAD is detached", () => {
    expect(labels(syncRows("", "", () => {}))).toEqual(["Fetch", "Pull (off)", "Push (off)"]);
  });

  it("offers nothing while one runs", () => {
    expect(labels(syncRows("main", "push", () => {}))).toEqual(["Fetch (off)", "Pull (off)", "Push (off)"]);
  });

  it("runs the action picked", () => {
    const ran: string[] = [];
    const [, pull] = syncRows("main", "", (action) => ran.push(action));
    if (pull && "onSelect" in pull) pull.onSelect();
    expect(ran).toEqual(["pull"]);
  });
});
