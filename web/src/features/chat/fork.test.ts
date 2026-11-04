import { describe, expect, it } from "vitest";
import { canFork, forkLabel, forkedWords, FORKABLE } from "./fork";

describe("canFork", () => {
  it("is the agents whose own CLI starts on a copy of a session", () => {
    expect([...FORKABLE].sort()).toEqual(["claude", "codex"]);
    expect(canFork("claude")).toBe(true);
    expect(canFork("codex")).toBe(true);
    expect(canFork("gemini")).toBe(false);
    expect(canFork("kimi")).toBe(false);
    expect(canFork("")).toBe(false);
    expect(canFork(undefined)).toBe(false);
  });
});

describe("the words", () => {
  it("name the agent in the menu row and the toasts", () => {
    expect(forkLabel("claude")).toBe("Fork Claude Code into a new tab");
    expect(forkLabel("zzz")).toBe("Fork zzz into a new tab");
    expect(forkedWords("codex")).toEqual({ text: "Forked: Codex goes on from a copy of this conversation in a new tab", error: false });
    expect(forkedWords("claude", "agent_pane_busy")).toEqual({ text: "The tab is open, but Claude Code did not start on the copy: agent_pane_busy", error: true });
  });
});
