import { describe, expect, it } from "vitest";
import { broadcast, picked, statusWords } from "./broadcast";
import type { AgentTarget } from "./targets";

const target = (paneId: string, status: string): AgentTarget => ({ machineId: "local", machineLabel: "local", sessionName: "ws", paneTitle: paneId, paneId, agent: "claude", status, cwd: "/w" });
const all = [target("a", "run"), target("b", "idle"), target("c", "done"), target("d", "input")];

describe("picked", () => {
  it("ticks by state and never an agent asking a question", () => {
    expect([...picked(all, "all")]).toEqual(["local|a", "local|b", "local|c"]);
    expect([...picked(all, "working")]).toEqual(["local|a"]);
    expect([...picked(all, "resting")]).toEqual(["local|b", "local|c"]);
    expect([...picked(all, "none")]).toEqual([]);
  });
});

describe("broadcast", () => {
  it("sends in turn and reports each, going on past a refusal", async () => {
    const sent: string[] = [];
    const out = await broadcast(all.slice(0, 3), "status?", async (t) => {
      if (t.paneId === "b") throw new Error("409 agent busy");
      sent.push(t.paneId);
    });
    expect(sent).toEqual(["a", "c"]);
    expect(out.map((o) => [o.target.paneId, o.error ?? "ok"])).toEqual([
      ["a", "ok"],
      ["b", "409 agent busy"],
      ["c", "ok"],
    ]);
  });
});

describe("statusWords", () => {
  it("says the state as the picker does", () => {
    expect(["run", "input", "done", "idle", "shell"].map(statusWords)).toEqual(["working", "asking — would refuse", "done", "idle", "idle"]);
  });
});
