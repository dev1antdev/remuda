import { describe, expect, it } from "vitest";
import type { Subagent, Turn } from "./api";
import { sameSubagents, subagentLine, subagentWords, subagentsOf } from "./subagents";

const working: Subagent = { id: "agent-a1", tool_id: "toolu_1", description: "PR: quiet hours", kind: "fork", status: "working", tools: 4, doing: { type: "tool", tool_name: "Bash", input: { command: "go test ./..." } } };
const done: Subagent = { id: "agent-a2", tool_id: "toolu_2", description: "Find the parser", kind: "Explore", status: "done", tools: 9, failed: 1, report: "## Found\n\nThe parser lives in `claude.go`.\n\nMore below.", took: 42 };
const quiet: Subagent = { id: "agent-a3", tool_id: "toolu_3", description: "Stalled", status: "quiet", tools: 2, report: "Starting on it." };

const turn = (...ids: string[]): Turn => ({ role: "assistant", parts: ids.map((id) => ({ type: "tool", tool_name: "Agent", tool_id: id, input: {} })) });

describe("subagentsOf", () => {
  it("matches the turn's Agent calls by tool id, in the turn's order", () => {
    expect(subagentsOf(turn("toolu_2", "toolu_1"), [working, done, quiet]).map((s) => s.id)).toEqual(["agent-a2", "agent-a1"]);
    expect(subagentsOf(turn("toolu_9"), [working])).toEqual([]);
    expect(subagentsOf({ role: "user", parts: [{ type: "text", text: "go" }] }, [working])).toEqual([]);
    expect(subagentsOf(turn("toolu_1"), undefined)).toEqual([]);
  });
});

describe("sameSubagents", () => {
  it("is true for another reading that shows the same rows", () => {
    expect(sameSubagents([working, done], [{ ...working, ended: "later" }, { ...done }])).toBe(true);
  });
  it("is false when a status, a count, the call in flight or the report moved", () => {
    expect(sameSubagents([working], [{ ...working, status: "done" }])).toBe(false);
    expect(sameSubagents([working], [{ ...working, tools: 5 }])).toBe(false);
    expect(sameSubagents([working], [{ ...working, doing: { type: "tool", tool_name: "Read", input: { file_path: "/x" } } }])).toBe(false);
    expect(sameSubagents([done], [{ ...done, report: "Other" }])).toBe(false);
    expect(sameSubagents([working], [])).toBe(false);
  });
});

describe("subagentLine", () => {
  it("names the call in flight while the agent works", () => {
    expect(subagentLine(working)).toBe("Bash · go test ./...");
  });
  it("is the first line of the report, marks stripped, once done or without a call", () => {
    expect(subagentLine(done)).toBe("Found");
    expect(subagentLine(quiet)).toBe("Starting on it.");
    expect(subagentLine({ ...working, doing: undefined, report: undefined })).toBe("");
  });
});

describe("subagentWords", () => {
  it("says what the lead waits on", () => {
    expect(subagentWords([working, done], true)).toBe("waiting on 1 subagent");
    expect(subagentWords([working, { ...working, id: "b" }], true)).toBe("waiting on 2 subagents");
    expect(subagentWords([], true)).toBe("waiting on a subagent");
    expect(subagentWords(undefined, true)).toBe("waiting on a subagent");
  });
  it("counts the ones still at work otherwise, nothing when none is", () => {
    expect(subagentWords([working, done, quiet], false)).toBe("1 subagent working");
    expect(subagentWords([working], false)).toBe("1 subagent working");
    expect(subagentWords([working, { ...working, id: "b" }], false)).toBe("2 subagents working");
    expect(subagentWords([done, quiet], false)).toBe("");
    expect(subagentWords(undefined, false)).toBe("");
  });
});
