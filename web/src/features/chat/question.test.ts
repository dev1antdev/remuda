import { describe, expect, it } from "vitest";
import { keysTo, questionLines } from "./question";

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
