import { describe, expect, it } from "vitest";
import type { Risk } from "@/features/roster/api";
import { levelWords, pathsWords, reasonWords, riskChip, riskHead, riskTip, scoreOf } from "./risk";

describe("the review's risk in words", () => {
  it("names the first paths and counts the rest", () => {
    expect(pathsWords(["a.go", "b.go", "c.go"], 7)).toBe("a.go, b.go, c.go and 4 more");
    expect(pathsWords(["a.go"])).toBe("a.go");
    expect(pathsWords(undefined, 0)).toBe("");
  });

  it("says every reason the server gives", () => {
    expect(reasonWords({ kind: "size", points: 25, count: 30, lines: 900 })).toBe("A big change: about 900 lines in 30 files");
    expect(reasonWords({ kind: "size", points: 10, count: 1, lines: 160 })).toBe("A sizeable change: about 160 lines in 1 file");
    expect(reasonWords({ kind: "untested", points: 10, count: 2, paths: ["a.go", "b.go"] })).toBe("2 source files changed with no test beside them: a.go, b.go");
    expect(reasonWords({ kind: "untested", points: 5, count: 1, paths: ["a.go"] })).toBe("1 source file changed with no test beside it: a.go");
    expect(reasonWords({ kind: "migration", points: 15, count: 1, paths: ["db/migrations/1.sql"] })).toBe("A migration or schema: db/migrations/1.sql");
    expect(reasonWords({ kind: "lockfile", points: 5, count: 1, paths: ["go.sum"] })).toBe("The dependencies change: go.sum");
    expect(reasonWords({ kind: "ci", points: 10, count: 1, paths: [".github/workflows/ci.yml"] })).toBe("What CI runs changes: .github/workflows/ci.yml");
    expect(reasonWords({ kind: "auth", points: 15, count: 4, paths: ["a", "b", "c"] })).toBe("Sign-in, permissions or secrets code: a, b, c and 1 more");
    expect(reasonWords({ kind: "secrets", points: 40, count: 2 })).toBe("The secret check found 2 possible secrets");
    expect(reasonWords({ kind: "checks", points: 20, count: 1 })).toBe("1 check fails on its pull request");
    expect(reasonWords({ kind: "conflicts", points: 15 })).toBe("Its pull request has conflicts");
    expect(reasonWords({ kind: "changes", points: 10 })).toBe("A reviewer asked for changes on its pull request");
    expect(reasonWords({ kind: "collision", points: 20, same: true, with: "web" })).toBe("Changes the same lines as web");
    expect(reasonWords({ kind: "collision", points: 8, with: "web" })).toBe("Changes the same files as web");
    expect(reasonWords({ kind: "someday", points: 1 })).toBe("someday");
  });

  it("is a chip with its score and a tooltip with every reason and its points", () => {
    const risk: Risk = { score: 45, level: "medium", reasons: [{ kind: "secrets", points: 40, count: 1 }, { kind: "lockfile", points: 5, count: 1, paths: ["go.sum"] }] };
    expect(riskChip(risk)).toBe("risk 45");
    expect(riskTip(risk)).toBe("Risk 45 of 100: medium risk\n· The secret check found 1 possible secret (+40)\n· The dependencies change: go.sum (+5)");
    expect(riskTip({ score: 0, level: "low", reasons: [] })).toBe("Risk 0 of 100: low risk\nA small change with nothing that asks for a second look.");
    expect(riskHead(risk)).toBe("Risk 45 of 100: medium risk");
    expect(levelWords("high")).toBe("high risk");
    expect(scoreOf(undefined)).toBe(0);
    expect(scoreOf(risk)).toBe(45);
  });
});
