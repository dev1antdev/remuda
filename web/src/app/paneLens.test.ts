import { describe, expect, it } from "vitest";
import { keepsTerminal, lensOn, lensesOf } from "./paneLens";

describe("a pane's lenses", () => {
  it("gives an agent the three and a shell its terminal alone", () => {
    expect(lensesOf("claude")).toEqual(["chat", "terminal", "changes"]);
    expect(lensesOf("")).toEqual(["terminal"]);
  });
  it("shows a shell its terminal whatever was asked, an agent the lens asked", () => {
    expect(lensOn("chat", "")).toBe("terminal");
    expect(lensOn("changes", "")).toBe("terminal");
    expect(lensOn("terminal", "")).toBe("terminal");
    expect(lensOn("chat", "codex")).toBe("chat");
    expect(lensOn("changes", "claude")).toBe("changes");
  });
  it("keeps the terminal of a shell an agent starts in, not of an agent's pane a restart passed through a shell", () => {
    expect(keepsTerminal(true, "claude")).toBe(true);
    expect(keepsTerminal(true, "")).toBe(false);
    expect(keepsTerminal(false, "claude")).toBe(false);
  });
});
