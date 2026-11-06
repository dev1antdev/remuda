import { describe, expect, it } from "vitest";
import { ALL_REVEALED, FIRST_TURNS, hiddenCount, MOUNT_STEP, nextReveal } from "./mount";

describe("progressive mount", () => {
  it("hides nothing of a short conversation", () => {
    expect(hiddenCount(12, 0)).toBe(0);
    expect(hiddenCount(FIRST_TURNS, 0)).toBe(0);
  });
  it("hides all but the last screenful of a long one, less what was revealed", () => {
    expect(hiddenCount(1680, 0)).toBe(1680 - FIRST_TURNS);
    expect(hiddenCount(1680, 500)).toBe(1680 - FIRST_TURNS - 500);
    expect(hiddenCount(1680, 1680)).toBe(0);
  });
  it("keeps new turns at the end visible: the count hides from the start", () => {
    const revealedAll = 1680;
    expect(hiddenCount(1685, revealedAll)).toBe(0);
  });
  it("reveals a step at a time, and everything once a slice reaches the start", () => {
    expect(nextReveal(1680, 0)).toBe(MOUNT_STEP);
    expect(nextReveal(1680, 1000)).toBe(1000 + MOUNT_STEP);
    expect(nextReveal(1680, 1400)).toBe(ALL_REVEALED);
    expect(nextReveal(1680, 1480)).toBe(ALL_REVEALED);
    expect(nextReveal(50, 0)).toBe(ALL_REVEALED);
  });
  it("hides no turn again once the log was mounted to its start, however many arrive", () => {
    const revealed = nextReveal(1680, 1400);
    expect(hiddenCount(1680, revealed)).toBe(0);
    expect(hiddenCount(1681, revealed)).toBe(0);
    expect(hiddenCount(5000, revealed)).toBe(0);
  });
  it("hides only the turn that pushed a just-crossed short conversation past the first screenful, once", () => {
    expect(hiddenCount(FIRST_TURNS + 1, 0)).toBe(1);
    expect(hiddenCount(FIRST_TURNS + 50, nextReveal(FIRST_TURNS + 1, 0))).toBe(0);
  });
});
