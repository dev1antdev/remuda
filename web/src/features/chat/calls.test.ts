import { describe, expect, it } from "vitest";
import type { Turn } from "./api";
import { callStats, callWords } from "./calls";

const turn = (...parts: Turn["parts"]): Turn => ({ role: "assistant", parts });
const bash = (command: string, extra: Partial<Turn["parts"][number]> = {}) => ({ type: "tool" as const, tool_name: "Bash", input: { command }, output: "", ...extra });

describe("callStats", () => {
  it("counts failed calls", () => {
    expect(callStats(turn(bash("go test", { is_error: true }), bash("ls"), { type: "text", text: "done" }))).toEqual({ failed: 1, repeated: 0 });
  });

  it("counts a call repeating an earlier one of the turn exactly, whatever came between", () => {
    expect(callStats(turn(bash("go test"), bash("ls"), bash("go test"), bash("go test")))).toEqual({ failed: 0, repeated: 2 });
    expect(callStats(turn({ type: "tool", tool_name: "Read", input: '{"file_path":"/a"}' }, { type: "tool", tool_name: "Read", input: '{"file_path":"/a"}' }))).toEqual({ failed: 0, repeated: 1 });
  });

  it("tells tools and inputs apart and ignores text and thinking", () => {
    expect(callStats(turn(bash("ls"), { type: "tool", tool_name: "Read", input: { command: "ls" } }, bash("ls -l"), { type: "thinking", text: "ls" }))).toEqual({ failed: 0, repeated: 0 });
    expect(callStats(turn())).toEqual({ failed: 0, repeated: 0 });
  });
});

describe("callWords", () => {
  it("says what went wrong and nothing when nothing did", () => {
    expect(callWords({ failed: 0, repeated: 0 })).toBe("");
    expect(callWords({ failed: 2, repeated: 0 })).toBe("2 failed");
    expect(callWords({ failed: 1, repeated: 3 })).toBe("1 failed · 3 repeated");
  });
});
