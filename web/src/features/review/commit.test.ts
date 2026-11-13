import { describe, expect, it } from "vitest";
import { boxRows, commitAnywayAsk, commitHint, commitLabel, committedWords, leaveAmend, loadAmend, whyNotCommit, type CommitState } from "./commit";

const state = (more: Partial<CommitState> = {}): CommitState => ({ message: "Say hello", mode: "commit", staged: 1, toStage: 2, conflicts: 0, busy: "", amendHead: null, loadingAmend: false, working: false, ...more });

describe("the commit box", () => {
  it("commits what is staged, else everything listed", () => {
    expect(commitLabel(state())).toBe("Commit staged");
    expect(commitLabel(state({ staged: 0 }))).toBe("Commit all");
    expect(commitLabel(state({ mode: "amend" }))).toBe("Amend");
    expect(commitLabel(state({ working: true }))).toBe("Committing…");
    expect(commitHint(state({ staged: 0 }), false, false)).toBe("Stages the 2 listed files first.");
    expect(commitHint(state(), false, true)).toBe("The commit takes everything staged in the repository, not only this folder.");
  });

  it("says why it is off, the agent at work first", () => {
    expect(whyNotCommit(state())).toBe("");
    expect(whyNotCommit(state({ message: "  " }))).toBe("Write a message first.");
    expect(whyNotCommit(state({ staged: 0, toStage: 0 }))).toBe("Nothing to commit.");
    expect(whyNotCommit(state({ conflicts: 1 }))).toBe("Resolve the conflicts and stage the files before committing.");
    expect(whyNotCommit(state({ busy: "Codex", conflicts: 1 }))).toBe("Codex is working here: commit once it rests.");
    expect(whyNotCommit(state({ mode: "amend", staged: 0, toStage: 0, amendHead: "a".repeat(40) }))).toBe(""); // a message alone may be amended
    expect(whyNotCommit(state({ mode: "amend", loadingAmend: true }))).toBe("Loading the latest commit…");
  });

  it("loads the last message into an empty box only, and takes it back unless it was edited", () => {
    expect(loadAmend("", "Last subject")).toBe("Last subject");
    expect(loadAmend("my draft", "Last subject")).toBeNull();
    expect(leaveAmend("Last subject", "Last subject")).toBe("");
    expect(leaveAmend("Last subject, edited", "Last subject")).toBe("Last subject, edited");
    expect(leaveAmend("mine", null)).toBe("mine");
  });

  it("grows to eight rows", () => {
    expect([boxRows(""), boxRows("a\nb\nc"), boxRows("x\n".repeat(20))]).toEqual([2, 3, 8]);
  });

  it("asks before committing past the secret check", () => {
    expect(commitAnywayAsk("found", { findings: [{ file: "a", line: 1, rule: "jwt", excerpt: "" }], more: 1, files: 1, at: "" })).toBe("2 things in this work look like secrets. Commit them anyway?");
    expect(commitAnywayAsk("failed", null)).toMatch(/could not run/);
    expect(committedWords("1a2b3c4", "Say hello", false)).toBe("Committed 1a2b3c4: Say hello");
    expect(commitHint(state({ mode: "amend" }), true, false)).toMatch(/force push/);
  });
});
