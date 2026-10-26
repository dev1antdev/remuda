import { describe, expect, it } from "vitest";
import { keyLine, revokeKeyQuestion } from "./keys";

const now = Date.parse("2026-10-26T12:00:00Z");

describe("keyLine", () => {
  it("says what the key opens, when it was used and since when", () => {
    const line = keyLine({ scope: "tasks", created: "2026-10-20T08:00:00Z", last_used: "2026-10-26T09:00:00Z" }, now);
    expect(line.startsWith("tasks only · used 3 h ago · since ")).toBe(true);
  });

  it("knows a key never used", () => {
    expect(keyLine({ scope: "tasks", created: "2026-10-26T08:00:00Z", last_used: "2026-10-26T08:00:00Z" }, now)).toContain("never used");
  });

  it("names another scope as it is", () => {
    expect(keyLine({ scope: "everything", created: "2026-10-26T08:00:00Z", last_used: "2026-10-26T08:00:00Z" }, now).startsWith("everything")).toBe(true);
  });
});

describe("revokeKeyQuestion", () => {
  it("names the machine and what its agents lose", () => {
    const q = revokeKeyQuestion({ name: "GOG" });
    expect(q.text).toContain("“GOG”");
    expect(q.text).toContain("lose the tasks");
    expect(q.label).toBe("Revoke");
  });
});
