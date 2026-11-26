import { describe, expect, it } from "vitest";
import { keepsTurns } from "./partial";
import type { ChatResponse } from "./api";

const read = (session: string | undefined, turns: number): ChatResponse => ({
  session_id: session,
  sessions: [],
  turns: Array.from({ length: turns }, () => ({ role: "assistant", parts: [] })),
});

describe("a chat read that came back thinner", () => {
  it("keeps the live conversation's turns when the same session read empty", () => {
    expect(keepsTurns(read("s1", 3), read("s1", 0), null)).toBe(true);
  });
  it("takes an empty answer for a new session, an older one or a subagent", () => {
    expect(keepsTurns(read("s1", 3), read("s2", 0), null)).toBe(false);
    expect(keepsTurns(read("s1", 3), read("s1", 0), "s1")).toBe(false);
    expect(keepsTurns(read("s1", 3), read("s1", 0), null, "agent-1")).toBe(false);
    expect(keepsTurns(null, read("s1", 0), null)).toBe(false);
  });
  it("takes a read that still holds a turn, and does not hide a real answer", () => {
    expect(keepsTurns(read("s1", 3), read("s1", 1), null)).toBe(false);
    expect(keepsTurns(read("s1", 0), read("s1", 1), null)).toBe(false);
  });
});
