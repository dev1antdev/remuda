import { describe, expect, it } from "vitest";
import { revokeViewerQuestion, viewerLine } from "./viewerLinks";

const now = Date.parse("2026-10-29T12:00:00Z");

describe("viewerLine", () => {
  it("says when the link was used and since when", () => {
    expect(viewerLine({ created: "2026-10-20T08:00:00Z", last_used: "2026-10-29T09:00:00Z" }, now).startsWith("used 3 h ago · since ")).toBe(true);
  });
  it("knows a link nobody opened yet", () => {
    expect(viewerLine({ created: "2026-10-29T08:00:00Z", last_used: "2026-10-29T08:00:00Z" }, now)).toContain("never used");
  });
});

describe("revokeViewerQuestion", () => {
  it("names the link and what goes with it", () => {
    const q = revokeViewerQuestion({ name: "the office TV" });
    expect(q.text).toContain("the office TV");
    expect(q.text).toContain("signed out");
    expect(q.label).toBe("Revoke");
  });
});
