import { describe, expect, it } from "vitest";
import type { Part } from "./api";
import { callState, pieces } from "./toolRuns";

const tool = (name: string, extra: Partial<Part> = {}): Part => ({ type: "tool", tool_name: name, ...extra });
const text: Part = { type: "text", text: "x" };
const thinking: Part = { type: "thinking", text: "hmm" };
const sent: Part = { type: "queued", text: "also this" };

const shape = (parts: Part[]) => pieces(parts).map((p) => (p.kind === "card" ? `[${p.items.map((i) => i.index).join(",")}]` : String(p.item.index)));

describe("tool cards", () => {
  it("draws a run of calls as one card, the thinking between them inside", () => {
    expect(shape([text, tool("Read"), tool("Edit"), text])).toEqual(["0", "[1,2]", "3"]);
    expect(shape([thinking, tool("Read"), thinking, tool("Bash"), thinking, text])).toEqual(["0", "[1,2,3]", "4", "5"]);
    expect(shape([tool("Read")])).toEqual(["[0]"]);
    expect(shape([tool("Read"), sent, tool("Bash")])).toEqual(["[0]", "1", "[2]"]);
    expect(shape([tool("Read"), text, tool("Bash"), tool("Edit")])).toEqual(["[0]", "1", "[2,3]"]);
    expect(shape([])).toEqual([]);
  });

  it("says how each call went", () => {
    expect(callState(tool("Bash", { output: "ok" }), false)).toBe("done");
    expect(callState(tool("Bash", { output: "", is_error: true }), true)).toBe("failed");
    expect(callState(tool("Bash"), true)).toBe("running");
    expect(callState(tool("Bash"), false)).toBe("unanswered");
  });
});
