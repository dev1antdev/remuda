import { describe, expect, it } from "vitest";
import type { Part, Turn } from "./api";
import { chips, LOOP_FROM, SQUEEZE_MS, squeezer, summary, timeline } from "./timeline";

const T0 = Date.parse("2026-10-24T10:00:00.000Z");
const at = (s: number) => new Date(T0 + s * 1000).toISOString();
const user = (s: number, text: string, id = `u${s}`): Turn => ({ role: "user", ts: at(s), ids: [id], parts: [{ type: "text", text }] });
const answer = (s: number, took: number, parts: Part[], id = `a${s}`): Turn => ({ role: "assistant", ts: at(s), ids: [id], took, parts });
const call = (name: string, input: unknown, extra: Partial<Part> = {}): Part => ({ type: "tool", tool_name: name, input, output: "ok", ...extra });

describe("timeline", () => {
  it("lays one segment per prompt and its answer, in time, with the prompt's first line", () => {
    const line = timeline([user(0, "Fix the tests\n\nmore"), answer(1, 59, [call("Bash", { command: "go test" })]), user(120, "Thanks"), answer(121, 10, [])]);
    expect(line.segments.map((s) => [s.id, s.prompt, s.took])).toEqual([
      ["u0", "Fix the tests", 59],
      ["u120", "Thanks", 10],
    ]);
    expect(line.segments[0]).toMatchObject({ start: T0, end: T0 + 59_000, x: 0 });
    expect(line.segments[1]!.x).toBeCloseTo(120 / 130);
    expect(line.segments[1]!.w).toBeCloseTo(10 / 130);
    expect(line.total).toBe(130);
    expect(line.highlights).toEqual([]);
    expect(line.waits).toBe(0);
  });

  it("squeezes a gap over ten minutes to ten, so a break does not flatten the day", () => {
    const hour = 3600;
    const line = timeline([user(0, "a"), answer(1, 60, []), user(2 * hour, "b"), answer(2 * hour + 1, 60, [])]);
    // 60 s of work, a two-hour gap drawn as 600 s, 60 s of work: the second prompt sits at 660 of 720
    expect(line.segments[1]!.x).toBeCloseTo(660 / 720);
    expect(line.waits).toBe(1);
    expect(summary(line)).toBe("2 h 1 min from the first prompt to the end of the last answer, 1 break over 30 min; a gap over 10 min is drawn as 10");
    const place = squeezer([0, 1000, 1000 + SQUEEZE_MS * 3, 1000 + SQUEEZE_MS * 3 + 1000]);
    expect(place(500)).toBeCloseTo(500 / (2000 + SQUEEZE_MS));
    expect(place(1000 + SQUEEZE_MS * 3)).toBeCloseTo((1000 + SQUEEZE_MS) / (2000 + SQUEEZE_MS));
    expect(place(-5)).toBe(0);
    expect(place(10 ** 15)).toBe(1);
    expect(squeezer([])(5)).toBe(0);
  });

  it("marks the first failed call, with how many failed in all", () => {
    const line = timeline([
      user(0, "go"),
      answer(1, 30, [call("Bash", { command: "go test ./..." }, { is_error: true, at: at(5) }), call("Edit", { file_path: "/w/a.go" }, { is_error: true, at: at(9) })]),
    ]);
    expect(line.highlights).toEqual([expect.objectContaining({ kind: "error", id: "a1", at: T0 + 5000, label: "First error", count: 2, detail: "Bash go test ./... · 2 failed calls in all" })]);
    expect(line.highlights[0]!.x).toBeCloseTo(5 / 30);
    expect(chips(line)).toEqual([{ kind: "error", id: "a1", text: "First of 2 errors", title: "Bash go test ./... · 2 failed calls in all" }]);
  });

  it("marks the longest call with its words and length", () => {
    const line = timeline([user(0, "go"), answer(1, 300, [call("Read", { file_path: "/w/a.go" }, { took: 0.4, at: at(2) }), call("Bash", { command: "make web" }, { took: 95, at: at(3) })])]);
    expect(line.highlights).toEqual([expect.objectContaining({ kind: "longest", at: T0 + 3000, label: "Longest call 1 min 35 s", detail: "Bash make web" })]);
    expect(chips(line)).toEqual([{ kind: "longest", id: "a1", text: "Longest call 1 min 35 s", title: "Bash make web" }]);
  });

  it("marks a loop of identical calls from three on, and a new prompt breaks the run", () => {
    const same = () => call("Grep", { pattern: "x", path: "/w" }, { at: at(2) });
    const clean = timeline([user(0, "go"), answer(1, 10, Array.from({ length: LOOP_FROM - 1 }, same))]);
    expect(clean.highlights).toEqual([]);
    const looping = timeline([user(0, "go"), answer(1, 10, [...Array.from({ length: 4 }, same), call("Grep", { pattern: "y", path: "/w" })])]);
    expect(looping.highlights).toEqual([expect.objectContaining({ kind: "loop", id: "a1", label: "Loop ×4", count: 4, detail: "Grep /w: 4 identical calls in a row" })]);
    const split = timeline([user(0, "go"), answer(1, 10, [same(), same()]), user(20, "again"), answer(21, 10, [same()])]);
    expect(split.highlights).toEqual([]);
    expect(chips(looping).map((c) => c.text)).toEqual(["Loop ×4"]);
  });

  it("marks every question and a compaction, one chip for each kind", () => {
    const line = timeline([
      user(0, "This session is being continued from a previous conversation that ran out of context."),
      answer(1, 20, [call("AskUserQuestion", { questions: [] }, { at: at(5) })]),
      user(30, "b"),
      answer(31, 20, [call("AskUserQuestion", { questions: [] }, { at: at(40) })]),
    ]);
    expect(line.highlights.map((h) => h.kind)).toEqual(["compaction", "question", "question"]);
    expect(chips(line)).toEqual([
      { kind: "question", id: "a1", text: "2 questions", title: "the agent asked 2 times; this goes to the first" },
      { kind: "compaction", id: "u0", text: "Compacted", title: "the conversation went on in a new file, from a summary of the one before" },
    ]);
  });

  it("orders the marks by time whatever found them, and skips turns without a stamp", () => {
    const line = timeline([
      { role: "user", parts: [{ type: "text", text: "no stamp" }] },
      user(0, "go"),
      answer(1, 100, [call("Bash", { command: "slow" }, { took: 50, at: at(2) }), call("Bash", { command: "boom" }, { is_error: true, at: at(60) })]),
    ]);
    expect(line.segments).toHaveLength(1);
    expect(line.highlights.map((h) => h.kind)).toEqual(["longest", "error"]);
    expect(chips(line).map((c) => c.kind)).toEqual(["error", "longest"]);
  });

  it("is empty on a clean, stampless or one-turn conversation", () => {
    expect(timeline([])).toEqual({ segments: [], highlights: [], waits: 0, total: 0 });
    expect(timeline([user(0, "hi")]).segments).toEqual([]);
    expect(chips(timeline([user(0, "hi"), answer(1, 5, [call("Read", { file_path: "/w/a" })])]))).toEqual([]);
  });
});
