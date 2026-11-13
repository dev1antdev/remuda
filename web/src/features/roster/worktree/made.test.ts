import { describe, expect, it } from "vitest";
import { madeWords } from "./made";

describe("the words after a create", () => {
  it("say what happened, a line each", () => {
    expect(madeWords({ pane: "w2:p1", opened: true, notes: [] })).toEqual(["Its workspace was open already: it is in front now."]);
    expect(madeWords({ pane: "w2:p1", opened: false, setup: "first", warning: "Created, but the agent failed to start: x", notes: ["git config: locked"] })).toEqual([
      "The setup script runs first in the new worktree; the agent starts once it is done.",
      "Created, but the agent failed to start: x",
      "git config: locked",
    ]);
    expect(madeWords({ pane: "w2:p1", opened: false, notes: [] })).toEqual([]);
  });
});
