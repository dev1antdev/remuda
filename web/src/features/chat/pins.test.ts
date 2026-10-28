import { describe, expect, it } from "vitest";
import type { Turn } from "./api";
import { MAX_PINS, isPinned, pinKey, pinLine, pinOf, pinWords, shortSession, togglePin, type Pin } from "./pins";

const answer = (ids: string[], ...parts: Turn["parts"]): Turn => ({ role: "assistant", ids, parts });
const prompt = (ids: string[], text: string): Turn => ({ role: "user", ids, parts: [{ type: "text", text }] });
const pin = (turn: string, session = "s-1"): Pin => ({ session, turn, line: turn, at: "2026-10-27T10:00:00.000Z", kind: "answer" });

describe("pinKey", () => {
  it("is one pref per machine and folder", () => {
    expect(pinKey("local", "/Users/me/remuda")).toBe("pins:local|/Users/me/remuda");
    expect(pinKey("gog", "/home/me/x")).not.toBe(pinKey("local", "/home/me/x"));
  });
});

describe("togglePin", () => {
  it("puts a new pin first and takes a pinned turn out", () => {
    const one = togglePin([], pin("a"));
    const two = togglePin(one, pin("b"));
    expect(two.map((p) => p.turn)).toEqual(["b", "a"]);
    expect(isPinned(two, "a")).toBe(true);
    expect(togglePin(two, pin("a")).map((p) => p.turn)).toEqual(["b"]);
    expect(isPinned(togglePin(two, pin("a")), "a")).toBe(false);
  });
  it("keeps at most MAX_PINS, the oldest going", () => {
    let pins: Pin[] = [];
    for (let i = 0; i < MAX_PINS + 5; i++) pins = togglePin(pins, pin(`t${i}`));
    expect(pins).toHaveLength(MAX_PINS);
    expect(pins[0]!.turn).toBe(`t${MAX_PINS + 4}`);
    expect(isPinned(pins, "t0")).toBe(false);
  });
});

describe("pinOf", () => {
  it("names the turn by its first record, the session shown and when, and knows a prompt from an answer", () => {
    const now = new Date("2026-10-27T12:00:00.000Z");
    expect(pinOf(answer(["u-1", "u-2"], { type: "text", text: "## Done\n\nAll green." }), "s-9", now)).toEqual({ session: "s-9", turn: "u-1", line: "Done", at: "2026-10-27T12:00:00.000Z", kind: "answer" });
    expect(pinOf(prompt(["p-1"], "fix the build"), "s-9", now)?.kind).toBe("prompt");
  });
  it("is nothing for a turn without records", () => {
    expect(pinOf({ role: "assistant", parts: [{ type: "text", text: "x" }] }, "s", new Date())).toBeNull();
    expect(pinOf(answer([], { type: "text", text: "x" }), "s", new Date())).toBeNull();
  });
});

describe("pinLine", () => {
  it("is the first line of the text the turn ended with", () => {
    expect(pinLine(answer(["a"], { type: "text", text: "Looking." }, { type: "tool", tool_name: "Bash", input: {}, output: "ok" }, { type: "text", text: "**All green** now.\nMore." }))).toBe("All green now.");
  });
  it("falls back to any text, then to the tools called, then to a word", () => {
    expect(pinLine(answer(["a"], { type: "text", text: "Looking at it." }, { type: "tool", tool_name: "Read", input: {}, output: "" }))).toBe("Looking at it.");
    expect(pinLine(answer(["a"], { type: "tool", tool_name: "Read", input: {}, output: "" }, { type: "tool", tool_name: "Edit", input: {}, output: "" }, { type: "tool", tool_name: "Read", input: {}, output: "" }))).toBe("(Read, Edit)");
    expect(pinLine(answer(["a"], { type: "thinking", text: "hmm" }))).toBe("(nothing said)");
  });
  it("cuts a long prompt at a word", () => {
    const line = pinLine(prompt(["p"], "word ".repeat(60)));
    expect(line.length).toBeLessThanOrEqual(121);
    expect(line.endsWith("…")).toBe(true);
  });
});

describe("pinWords", () => {
  const now = Date.parse("2026-10-27T13:00:00.000Z");
  it("says when, and names a session other than the one shown", () => {
    expect(pinWords(pin("a", "492634a3-456d-44b4-86e5-8515ba11581d"), "492634a3-456d-44b4-86e5-8515ba11581d", now)).toBe("3 h ago");
    expect(pinWords(pin("a", "492634a3-456d-44b4-86e5-8515ba11581d"), "other", now)).toBe("3 h ago · session 492634a3");
    expect(pinWords(pin("a", "492634a3-456d-44b4-86e5-8515ba11581d"), null, now)).toBe("3 h ago · session 492634a3");
    expect(shortSession("492634a3-456d-44b4-86e5-8515ba11581d")).toBe("492634a3");
  });
});
