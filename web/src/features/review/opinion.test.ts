import { describe, expect, it } from "vitest";
import type { DiffRow } from "./hunks";
import { adoptedText, asksPerson, byFile, headWords, offRows, onRow, openComments, reviewerFor, subWords, unlisted, type Opinion, type OpinionComment } from "./opinion";

const comment = (id: number, path: string, line: number, more: Partial<OpinionComment> = {}): OpinionComment => ({ id, path, line, text: `note ${id}`, ...more });
const row = (kind: DiffRow["kind"], oldLine: number | null, newLine: number | null): DiffRow => ({ kind, oldLine, newLine, text: `${kind === "add" ? "+" : kind === "del" ? "-" : " "}x` }) as DiffRow;
const opinion = (more: Partial<Opinion> = {}): Opinion => ({ machine: "local", dir: "/w/app", agent: "codex", tab: "w1:t2", pane: "w1:p2", asked: "", state: "done", comments: [], close: true, ...more });

describe("reviewerFor", () => {
  it("asks the other vendor: Codex for Claude Code, Claude Code for anyone else", () => {
    const kinds = ["claude", "codex", "gemini"];
    expect(reviewerFor("claude", kinds)).toBe("codex");
    expect(reviewerFor("codex", kinds)).toBe("claude");
    expect(reviewerFor("gemini", kinds)).toBe("claude");
    expect(reviewerFor("", kinds)).toBe("claude");
    expect(reviewerFor("claude", ["claude", "gemini"])).toBe("gemini");
    expect(reviewerFor("claude", ["claude"])).toBe("claude");
  });
});

describe("where the comments sit", () => {
  const rows = [row("hunk" as DiffRow["kind"], null, null), row("", 5, 5), row("del", 6, null), row("add", null, 6), row("", 7, 7)];
  it("puts a comment under the new file's line it names, never under a removed one", () => {
    expect(onRow(comment(1, "a.go", 6), rows[3]!)).toBe(true);
    expect(onRow(comment(1, "a.go", 6), rows[2]!)).toBe(false);
    expect(onRow(comment(1, "a.go", 5), rows[1]!)).toBe(true);
    expect(offRows([comment(1, "a.go", 6), comment(2, "a.go", 40)], rows).map((c) => c.id)).toEqual([2]);
  });
  it("keeps what is open, by file, and the files the lens does not list", () => {
    const op = opinion({ comments: [comment(1, "a.go", 6), comment(2, "b.go", 1, { dismissed: true }), comment(3, "a.go", 9), comment(4, "gone.go", 2)] });
    const open = openComments(op);
    expect(open.map((c) => c.id)).toEqual([1, 3, 4]);
    expect([...byFile(open).keys()]).toEqual(["a.go", "gone.go"]);
    expect(unlisted(open, ["a.go", "b.go"]).map((c) => c.id)).toEqual([4]);
    expect(openComments(null)).toEqual([]);
  });
});

describe("the words", () => {
  it("say who is at it and what stands", () => {
    expect(headWords(opinion({ state: "running" }), "Codex")).toMatch(/^Codex is reviewing/);
    expect(headWords(opinion({ state: "running" }), "Codex", true)).toBe("Codex is asking you something in its tab");
    expect(asksPerson({ status: "input" })).toBe(true);
    expect(asksPerson({ status: "run", radar: { state: "blocked" } })).toBe(true);
    expect(asksPerson({ status: "done", radar: { state: "done" } })).toBe(false);
    expect(asksPerson(undefined)).toBe(false);
    expect(headWords(opinion({ state: "unreadable" }), "Codex")).toMatch(/could not be read/);
    expect(headWords(opinion({ state: "gone", note: "the reviewer did not start: x" }), "Codex")).toBe("the reviewer did not start: x");
    expect(headWords(opinion(), "Codex")).toBe("Codex found nothing to comment on");
    expect(headWords(opinion({ comments: [comment(1, "a", 1), comment(2, "a", 2)] }), "Codex")).toBe("2 comments");
    expect(headWords(opinion({ comments: [comment(1, "a", 1, { dismissed: true })] }), "Codex")).toBe("every comment added or dismissed");
  });
  it("add to the line under the branch while an opinion stands", () => {
    expect(subWords(null)).toBe("");
    expect(subWords(opinion({ state: "running" }))).toBe("second opinion on its way");
    expect(subWords(opinion({ comments: [comment(1, "a", 1), comment(2, "a", 2), comment(3, "a", 3, { dismissed: true })] }))).toBe("second opinion: 2 comments");
    expect(subWords(opinion({ comments: [comment(1, "a", 1, { dismissed: true })] }))).toBe("");
    expect(subWords(opinion({ state: "unreadable" }))).toBe("");
  });
  it("mark an adopted comment with its severity and its reviewer", () => {
    expect(adoptedText(comment(1, "a.go", 3, { severity: "high", text: "a race" }), "Codex")).toBe("[high] a race — second opinion of Codex");
    expect(adoptedText(comment(1, "a.go", 3, { text: "a race" }), "Codex")).toBe("a race — second opinion of Codex");
  });
});
