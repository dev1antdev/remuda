import { describe, expect, it } from "vitest";
import { resumes } from "./query";

describe("resumes", () => {
  it("reads at once when a paused query comes back on, unless a read is already out", () => {
    expect(resumes(false, true, false)).toBe(true);
    expect(resumes(false, true, true)).toBe(false); // its changed inputs sent one in the same render
    expect(resumes(true, true, false)).toBe(false); // it never paused: the polls and the inputs carry it
    expect(resumes(true, false, false)).toBe(false);
    expect(resumes(false, false, false)).toBe(false);
  });
});
