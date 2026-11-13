import { describe, expect, it } from "vitest";
import { staleOf } from "./withdraw";

describe("the notifications taken back", () => {
  const shown = [{ tag: "local|w1:p1|input|1800000000000" }, { tag: "local|w1:p1|input|1800000090000" }, { tag: "digest|morning" }, { tag: "" }];
  it("are the ones shown under a withdrawn tag, never a newer episode of the same pane", () => {
    expect(staleOf(shown, ["local|w1:p1|input|1800000000000", "local|w2:p1|done|1800000000000"])).toEqual([shown[0]]);
  });
  it("are none when nothing was taken back, and never a notification without a tag", () => {
    expect(staleOf(shown, [])).toEqual([]);
    expect(staleOf(shown, [""])).toEqual([]);
  });
});
