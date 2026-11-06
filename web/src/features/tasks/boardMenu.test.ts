import { describe, expect, it } from "vitest";
import board from "./Board.tsx?raw";

// a hand-made menu at a fixed point opened past the window's bottom under a card the column had scrolled to (a look at
// 1440 × 900: the button at 825, the menu at 1066); ui/Menu opens where there is room and follows its button
describe("a board card's ⋯", () => {
  it("is a ui/Menu, never a hand-made menu at a fixed point", () => {
    expect(board).not.toMatch(/className="row-menu"/);
    expect(board).toMatch(/<Menu\b[\s\S]*label=\{tr\("Move to another column"\)\}/);
  });
});
