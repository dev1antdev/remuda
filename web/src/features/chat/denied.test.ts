import { describe, expect, it } from "vitest";
import { deniedHead, deniedKey, lastDenial, MAX_SHOWN, shownDenials, showsDenied } from "./denied";

const call = (n: number) => ({ tool: "Bash", what: `step ${n}`, at: `2026-11-12T03:0${n}:00Z`, tool_id: `toolu_${n}` });

describe("the denied calls' notice", () => {
  it("lists the first few and counts the rest", () => {
    const calls = Array.from({ length: MAX_SHOWN + 2 }, (_, i) => call(i));
    expect(shownDenials(calls).shown).toHaveLength(MAX_SHOWN);
    expect(shownDenials(calls).more).toBe(2);
    expect(shownDenials(calls.slice(0, 2))).toEqual({ shown: calls.slice(0, 2), more: 0 });
  });

  it("stands until dismissed, and again once another call is denied", () => {
    const calls = [call(1), call(2)];
    expect(showsDenied(calls, "")).toBe(true);
    expect(showsDenied(calls, lastDenial(calls))).toBe(false);
    expect(showsDenied([...calls, call(3)], lastDenial(calls))).toBe(true);
    expect(showsDenied([], "")).toBe(false);
    expect(showsDenied(undefined, "")).toBe(false);
    expect(lastDenial([{ tool: "Write", at: "t1" }])).toBe("t1|Write");
    expect(lastDenial([])).toBe("");
  });

  it("keeps a dismissal per machine and conversation, and counts in its head", () => {
    expect(deniedKey("gog", "5c1d")).toBe("denied.seen:gog|5c1d");
    expect(deniedHead(1)).toBe("Claude Code was denied a call nobody was there to approve:");
    expect(deniedHead(3)).toBe("Claude Code was denied 3 calls nobody was there to approve:");
  });
});
