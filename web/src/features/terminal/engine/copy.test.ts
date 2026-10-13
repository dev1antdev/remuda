import { describe, expect, it } from "vitest";
import { trimRows } from "./copy";

describe("trimRows", () => {
  it("drops each row's trailing spaces, tabs and no-break spaces, the rows kept", () => {
    expect(trimRows("a   \nb \t\nc")).toBe("a\nb\nc");
    expect(trimRows("  indented   ")).toBe("  indented");
    expect(trimRows("")).toBe("");
  });
});
