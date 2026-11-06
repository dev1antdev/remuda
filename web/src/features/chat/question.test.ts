import { describe, expect, it } from "vitest";
import { choosable, keysTo, pressInTurn, questionLines, sameDialog } from "./question";

const options = [
  { n: 1, label: "Yes", picked: false },
  { n: 2, label: "Yes, and don't ask again", picked: true },
  { n: 3, label: "No, and tell Claude what to do differently (esc)", picked: false },
];

describe("keysTo", () => {
  it("walks from the picked choice and confirms", () => {
    expect(keysTo(options, 1)).toEqual(["enter"]);
    expect(keysTo(options, 2)).toEqual(["down", "enter"]);
    expect(keysTo(options, 0)).toEqual(["up", "enter"]);
  });
  it("starts at the first choice when none is picked", () => {
    const none = options.map((o) => ({ ...o, picked: false }));
    expect(keysTo(none, 2)).toEqual(["down", "down", "enter"]);
  });
});

describe("sameDialog", () => {
  const offered = { text: "Bash command\n\nrm -rf build\n\nDo you want to proceed?", options };
  it("is the dialog the choice was offered on, wherever the cursor has moved since", () => {
    const moved = { ...offered, options: options.map((o, i) => ({ ...o, picked: i === 0 })) };
    expect(sameDialog(offered, moved)).toBe(true);
    // the cursor read again is where the walk starts: on the first choice now, "Yes" is one Enter away
    expect(keysTo(moved.options, 0)).toEqual(["enter"]);
  });
  it("is not another dialog that took its place, or the same words with other choices", () => {
    expect(sameDialog(offered, { ...offered, text: "Bash command\n\ngit push --force\n\nDo you want to proceed?" })).toBe(false);
    expect(sameDialog(offered, { ...offered, options: options.slice(0, 2) })).toBe(false);
    expect(sameDialog(offered, { ...offered, options: options.map((o, i) => (i === 1 ? { ...o, label: "Yes, allow all edits" } : o)) })).toBe(false);
  });
});

describe("pressInTurn", () => {
  it("stops at the first key refused, so a lost ↓ is never followed by the Enter that would confirm the wrong choice", async () => {
    const sent: string[] = [];
    const went = await pressInTurn(["down", "down", "enter"], (key) => {
      sent.push(key);
      return sent.length !== 2;
    });
    expect(went).toBe(false);
    expect(sent).toEqual(["down", "down"]);
  });
  it("sends every key when each goes, an answer of nothing counting as gone", async () => {
    const sent: string[] = [];
    expect(await pressInTurn(["up", "enter"], async (key) => void sent.push(key))).toBe(true);
    expect(sent).toEqual(["up", "enter"]);
  });
});

describe("choosable", () => {
  it("takes one press for a single choice, never for a multi-select", () => {
    expect(choosable({ text: "Proceed?", options })).toBe(true);
    expect(choosable({ text: "Which features?", options, multi: true })).toBe(false);
    expect(choosable({ text: "Nothing to pick", options: [] })).toBe(false);
  });
});

describe("questionLines", () => {
  it("heads with the first line, asks with the last, keeps the detail between", () => {
    expect(questionLines("Bash command\n\nrm -rf build\nRemove the build folder\n\nDo you want to proceed?")).toEqual({
      head: "Bash command",
      detail: "rm -rf build\nRemove the build folder",
      ask: "Do you want to proceed?",
    });
  });
  it("asks with a single line", () => {
    expect(questionLines("Do you want to proceed?")).toEqual({ head: "", detail: "", ask: "Do you want to proceed?" });
    expect(questionLines("")).toEqual({ head: "", detail: "", ask: "" });
  });
  it("has no detail between two lines", () => {
    expect(questionLines("Which storage?\nStorage")).toEqual({ head: "Which storage?", detail: "", ask: "Storage" });
  });
});
