import { describe, expect, it } from "vitest";
import { cut, isHead, labelsOf } from "./labels";
import { commitsWords, dateWords, laneColors, stampWords, stoppedWords, whereWords } from "./words";

describe("labelsOf", () => {
  const refs = [
    { kind: "tag" as const, name: "v1.0.0" },
    { kind: "remote" as const, name: "origin/fix" },
    { kind: "branch" as const, name: "fix" },
    { kind: "remote" as const, name: "upstream/feature/very-long-name-of-a-branch" },
    { kind: "branch" as const, name: "main", head: true },
  ];

  it("puts what is checked out first, a same-named remote on its branch, the rest alone", () => {
    const labels = labelsOf({ refs });
    expect(labels.map((l) => l.parts.map((p) => p.text).join(" | "))).toEqual(["main", "fix | origin", "upstream/feature/very…", "v1.0.0"]);
    expect(labels[0]!.head).toBe(true);
    expect(labels[2]!.parts[0]).toMatchObject({ kind: "remote", icon: true });
  });

  it("leaves the remotes out when they are not shown", () => {
    expect(labelsOf({ refs }, false).map((l) => l.kind)).toEqual(["branch", "branch", "tag"]);
  });

  it("marks a detached HEAD and knows the row HEAD stands on", () => {
    const labels = labelsOf({ refs: [{ kind: "tag", name: "v2" }, { kind: "head", name: "HEAD" }] });
    expect(labels[0]).toMatchObject({ kind: "head", head: true });
    expect(isHead({ refs: [{ kind: "head", name: "HEAD" }] })).toBe(true);
    expect(isHead({ refs: [{ kind: "branch", name: "x" }] })).toBe(false);
  });

  it("cuts a name at 22 with an ellipsis, by characters", () => {
    expect(cut("feature/abcdefghijklmnop")).toBe("feature/abcdefghijklm…");
    expect([...cut("ветка-с-очень-длинным-именем")].length).toBe(22);
    expect(cut("main")).toBe("main");
  });
});

describe("the lens's words", () => {
  const now = new Date(2026, 10, 13, 12, 0, 0).getTime();
  const ago = (s: number) => now / 1000 - s;

  it("says a commit's age as the date column does", () => {
    expect(dateWords(ago(30), now)).toBe("now");
    expect(dateWords(ago(12 * 60), now)).toBe("12m");
    expect(dateWords(ago(3 * 3600), now)).toBe("3h");
    expect(dateWords(ago(6 * 86400), now)).toBe("6d");
    expect(dateWords(new Date(2026, 9, 3).getTime() / 1000, now)).toBe("Oct 3");
    expect(dateWords(new Date(2025, 9, 3).getTime() / 1000, now)).toBe("Oct 2025");
  });

  it("says the moment in full, where HEAD stands and how many commits", () => {
    expect(stampWords(new Date(2026, 9, 28, 12, 9).getTime() / 1000)).toBe("2026-10-28 12:09");
    expect(whereWords("main", "abc")).toBe("main");
    expect(whereWords("", "0123456789abcdef")).toBe("detached at 01234567");
    expect(whereWords(undefined, undefined)).toBe("");
    expect(commitsWords(1, false)).toBe("1 commit");
    expect(commitsWords(300, true)).toBe("300+ commits");
    expect(stoppedWords("merge", 3)).toBe("A merge stopped on conflicts in 3 files.");
    expect(stoppedWords("rebase", 0)).toBe("A rebase is in progress.");
  });

  it("takes the lanes' colours from the theme's sixteen in git-graph's order", () => {
    const sixteen = Array.from({ length: 16 }, (_, i) => `#${i}`);
    expect(laneColors(sixteen).slice(0, 6)).toEqual(["#4", "#2", "#5", "#3", "#6", "#1"]);
    expect(laneColors(sixteen)).toHaveLength(12);
  });
});
