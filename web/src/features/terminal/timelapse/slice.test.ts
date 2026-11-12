import { describe, expect, it } from "vitest";
import { SLICE_MS, Slicer } from "./slice";

describe("Slicer", () => {
  it("is due once the slice has run its time, and starts the next one over", () => {
    let clock = 1000;
    const slice = new Slicer(() => clock);
    clock += SLICE_MS - 1;
    expect(slice.due).toBe(false);
    clock += 1;
    expect(slice.due).toBe(true);
    clock += 500; // the page's turn, however long it took
    slice.restart();
    expect(slice.due).toBe(false);
    clock += SLICE_MS;
    expect(slice.due).toBe(true);
  });
});
