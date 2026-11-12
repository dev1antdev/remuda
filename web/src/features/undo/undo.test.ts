import { describe, expect, it } from "vitest";
import type { Turn } from "@/features/chat/api";
import type { ListedTurn, UndoFile, UndoPlan } from "./api";
import { canRevert, defaultTurn, fileWords, matchTurns, nothingToDo, pushedWords, sentence, turnHint, turnLabel, undoable, undoneWords } from "./undo";

const at = (s: number) => new Date(Date.UTC(2026, 10, 12, 12, 0, s)).toISOString();
const kept = (over: Partial<ListedTurn> = {}): ListedTurn => ({ id: "t1", machine: "local", dir: "/repo", root: "/repo", pane: "w1:p1", agent: "Claude Code", started: at(0), ended: at(120), files: 0, size: 1, ...over });
const file = (over: Partial<UndoFile> = {}): UndoFile => ({ path: "a.go", was: "changed", dirty_at: false, dirty_to: true, ...over });

describe("the turn offered", () => {
  it("is the newest that can still be undone", () => {
    const turns = [kept({ id: "new", undone: { at: at(200), files: 1 } }), kept({ id: "big", why: "too big" }), kept({ id: "ok" })];
    expect(defaultTurn(turns)?.id).toBe("ok");
    expect(defaultTurn([kept({ id: "x", why: "no" })])?.id).toBe("x");
    expect(defaultTurn([])).toBeUndefined();
    expect(undoable(kept())).toBe(true);
  });
  it("goes by the agent's vendor name", () => {
    expect(turnLabel(kept({ agent: "claude" }))).toMatch(/^Claude Code · /);
    expect(turnLabel(kept({ agent: "" }))).toMatch(/^An agent · /);
  });
  it("reads the server's reason as a sentence", () => {
    expect(sentence("these files changed after the turn")).toBe("These files changed after the turn");
    expect(sentence("")).toBe("");
  });
  it("says its size, or why it is out of reach", () => {
    expect(turnHint(kept({ files: 2, commits: [{ sha: "abc", subject: "s", when: 1 }] }))).toBe("2 files · 1 commit · 2 min");
    expect(turnHint(kept({ why: "too big" }))).toBe("cannot be undone");
    expect(turnHint(kept({ undone: { at: at(9), files: 2 } }))).toBe("undone");
  });
});

describe("what happens to a file", () => {
  it("names the undo's move by what the turn did", () => {
    expect(fileWords(file({ was: "added" }), true)).toMatch(/removed/);
    expect(fileWords(file({ was: "deleted" }), true)).toBe("brought back");
    expect(fileWords(file({ was: "mode" }), false)).toBe("its mode put back");
    expect(fileWords(file(), false)).toBe("put back");
  });
  it("tells the committed files apart when the commits stay", () => {
    expect(fileWords(file({ committed: true, dirty_at: true }), false)).toMatch(/only reverting/);
    expect(fileWords(file({ committed: true, dirty_to: false }), false)).toBe("stays as the commits have it");
    expect(fileWords(file({ committed: true }), false)).toBe("back to the last commit");
    expect(fileWords(file({ committed: true, dirty_at: true }), true)).toBe("put back");
  });
});

describe("the plan", () => {
  const full = (over: Partial<ListedTurn> = {}) => ({ ...kept(over), files: [] });
  const plan = (over: Partial<UndoPlan> = {}): UndoPlan => ({ turn: full({ commits: [{ sha: "abc", subject: "s", when: 1 }] }), uncommitted: 0, ...over });
  it("reverts only commits git can revert now", () => {
    expect(canRevert(plan())).toBe(true);
    expect(canRevert(plan({ revert_why: "staged" }))).toBe(false);
    expect(canRevert(plan({ refused: { why: "changed" } }))).toBe(false);
    expect(canRevert(plan({ turn: full() }))).toBe(false);
    expect(canRevert(null)).toBe(false);
  });
  it("has nothing to do with the commits kept and nothing uncommitted", () => {
    expect(nothingToDo(plan(), false)).toBe(true);
    expect(nothingToDo(plan(), true)).toBe(false);
    expect(nothingToDo(plan({ uncommitted: 2 }), false)).toBe(false);
  });
  it("says what it did and what was pushed", () => {
    expect(undoneWords({ files: 3, reverted: ["a", "b"] })).toBe("Undone: 3 files put back, 2 commits reverted");
    expect(undoneWords({ files: 0, reverted: ["a"] })).toBe("Undone: 1 commit reverted");
    expect(undoneWords({ files: 1 })).toBe("Undone: 1 file put back");
    expect(pushedWords(["origin/main", "fork/main"])).toContain("origin/main, fork/main");
  });
});

describe("the answer a kept turn belongs to", () => {
  const conversation: Turn[] = [
    { role: "user", ts: at(0), ids: ["u1"], parts: [] },
    { role: "assistant", ts: at(5), ids: ["a1"], parts: [], took: 110 },
    { role: "user", ts: at(300), ids: ["u2"], parts: [] },
    { role: "assistant", ts: at(305), ids: ["a2"], parts: [], took: 50 },
  ];
  it("is the answer that ended within it, the pane's own", () => {
    const matched = matchTurns(conversation, [kept({ id: "second", started: at(299), ended: at(352) }), kept({ id: "first" }), kept({ id: "other", pane: "w2:p1" })], "w1:p1");
    expect(matched.get("a1")?.id).toBe("first");
    expect(matched.get("a2")?.id).toBe("second");
    expect(matched.size).toBe(2);
  });
  it("leaves an answer alone that no kept turn spans", () => {
    expect(matchTurns(conversation, [kept({ started: at(1000), ended: at(1100) })], "w1:p1").size).toBe(0);
  });
  it("gives an answer the newest kept turn only", () => {
    const matched = matchTurns(conversation, [kept({ id: "newer", started: at(0), ended: at(118) }), kept({ id: "older", started: at(0), ended: at(117) })], "w1:p1");
    expect(matched.get("a1")?.id).toBe("newer");
    expect(matched.size).toBe(1);
  });
});
