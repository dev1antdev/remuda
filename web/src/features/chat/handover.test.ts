import { describe, expect, it } from "vitest";
import { clampTurns, DEFAULT_TURNS, handedOverWords, handoverWords, MAX_TURNS, nextAgent, turnOptions } from "./handover";

describe("clampTurns", () => {
  it("keeps the count within what the server takes", () => {
    expect(clampTurns(3)).toBe(3);
    expect(clampTurns(-2)).toBe(0);
    expect(clampTurns(99)).toBe(MAX_TURNS);
    expect(clampTurns(2.6)).toBe(3);
    expect(clampTurns(NaN)).toBe(DEFAULT_TURNS);
  });
});

describe("turnOptions", () => {
  it("offers the recap alone and one to ten exchanges", () => {
    const options = turnOptions();
    expect(options).toHaveLength(MAX_TURNS + 1);
    expect(options[0]).toEqual({ value: 0, label: "The recap only" });
    expect(options[1]!.label).toBe("The last exchange");
    expect(options[3]!.label).toBe("The last 3 exchanges");
  });
});

describe("handoverWords", () => {
  it("says what goes over", () => {
    expect(handoverWords(0)).toBe("the recap of this conversation");
    expect(handoverWords(1)).toBe("the recap and the last exchange in full");
    expect(handoverWords(3)).toBe("the recap and the last 3 exchanges in full");
  });
});

describe("nextAgent", () => {
  it("offers another agent than the pane's first", () => {
    expect(nextAgent(["claude", "codex", "gemini"], "claude")).toBe("codex");
    expect(nextAgent(["claude"], "claude")).toBe("claude");
    expect(nextAgent([], "claude")).toBe("");
  });
});

describe("handedOverWords", () => {
  it("names the agent and rounds the size to whole kilobytes, one at least", () => {
    expect(handedOverWords("Codex", 14_300)).toBe("Handed over to Codex: 14 KB sent as its first prompt");
    expect(handedOverWords("Gemini", 200)).toBe("Handed over to Gemini: 1 KB sent as its first prompt");
  });
});
