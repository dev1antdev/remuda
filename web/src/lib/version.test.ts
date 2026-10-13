import { describe, expect, it } from "vitest";
import { newer } from "./version";

describe("newer", () => {
  it("compares x.y.z, whatever follows", () => {
    expect(newer("v0.4.0-24-g5dac31e", "v0.5.0")).toBe(true);
    expect(newer("v0.5.0", "v0.4.0-24-g5dac31e")).toBe(false);
    expect(newer("1.2.3", "1.2.10")).toBe(true);
    expect(newer("1.2.10", "1.2.3")).toBe(false);
  });
  it("is false for the same version and for anything without three numbers", () => {
    expect(newer("0.5.0", "0.5.0")).toBe(false);
    expect(newer("dev", "1.0.0")).toBe(false);
    expect(newer("1.0.0", "")).toBe(false);
  });
});
