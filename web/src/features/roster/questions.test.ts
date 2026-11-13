import { describe, expect, it } from "vitest";
import { MAX_ASKING, askedOf, questionBubble, sameQuestions } from "./questions";

describe("questionBubble", () => {
  it("holds up the dialog's head and the first line of its detail", () => {
    expect(questionBubble({ text: "Bash command\nrm -rf build\nclean the build\nDo you want to proceed?", options: [] })).toBe("Bash command · rm -rf build");
  });

  it("holds the asking line when the dialog has no head, nothing without a dialog", () => {
    expect(questionBubble({ text: "Which database?", options: [] })).toBe("Which database?");
    expect(questionBubble(null)).toBe("");
  });
});

describe("sameQuestions", () => {
  it("tells an unchanged reading of the questions from a new one", () => {
    const a = new Map([["local|w1:p1", "Bash command · rm -rf build"]]);
    expect(sameQuestions(a, new Map(a))).toBe(true);
    expect(sameQuestions(a, new Map([["local|w1:p1", "Bash command · ls"]]))).toBe(false);
    expect(sameQuestions(a, new Map())).toBe(false);
    expect(sameQuestions(new Map(), new Map())).toBe(true);
  });
});

describe("askedOf", () => {
  it("reads the first MAX_ASKING panes, and signs who they are so a round starts again only when that changes", () => {
    const askers = Array.from({ length: MAX_ASKING + 3 }, (_, i) => ({ key: `local|w${i}:p1`, machine: "local", pane: `w${i}:p1` }));
    const { wanted, signature } = askedOf(askers);
    expect(wanted).toHaveLength(MAX_ASKING);
    expect(signature).toBe(askedOf(askers.slice(0, MAX_ASKING)).signature);
    expect(askedOf([]).signature).toBe("");
  });
});
