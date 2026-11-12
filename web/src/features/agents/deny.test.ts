import { describe, expect, it } from "vitest";
import { canDenyPrompts, deniedCount, deniedList, denialLine, denyDefault, whyNoDeny, type Denial } from "./deny";

describe("the deny switch", () => {
  it("is Claude Code's alone, an unnamed agent being Claude Code", () => {
    expect(canDenyPrompts("claude")).toBe(true);
    expect(canDenyPrompts("")).toBe(true);
    expect(canDenyPrompts("codex")).toBe(false);
    expect(canDenyPrompts("gemini")).toBe(false);
  });

  it("starts on for a new record and keeps a saved one's word, off for one from before it", () => {
    expect(denyDefault(false, undefined)).toBe(true);
    expect(denyDefault(false, false)).toBe(false);
    expect(denyDefault(true, undefined)).toBe(false);
    expect(denyDefault(true, true)).toBe(true);
    expect(denyDefault(true, false)).toBe(false);
  });

  it("names the calls that were denied", () => {
    const calls: Denial[] = [{ tool: "Bash", what: "touch probe.txt" }, { tool: "Write", what: "/w/notes.txt" }, { tool: "WebSearch" }, { tool: "Bash", what: "git push" }];
    expect(deniedCount(1)).toBe("1 call denied");
    expect(deniedCount(4)).toBe("4 calls denied");
    expect(denialLine({ tool: "Bash", what: "touch probe.txt" })).toBe("Bash · touch probe.txt");
    expect(denialLine({ tool: "WebSearch" })).toBe("WebSearch");
    expect(deniedList(calls)).toBe("Bash · touch probe.txt; Write · /w/notes.txt; WebSearch; +1 more");
    expect(deniedList(calls.slice(0, 2))).toBe("Bash · touch probe.txt; Write · /w/notes.txt");
    expect(deniedList([])).toBe("");
  });

  it("says why it is disabled only when no agent it covers knows the setting", () => {
    expect(whyNoDeny([{ id: "claude", label: "Claude Code" }])).toBe("");
    expect(whyNoDeny([{ id: "codex", label: "Codex" }, { id: "claude", label: "Claude Code" }])).toBe("");
    expect(whyNoDeny([])).toBe("");
    expect(whyNoDeny([{ id: "codex", label: "Codex" }, { id: "codex", label: "Codex" }, { id: "gemini", label: "Gemini" }])).toBe(
      "Only Claude Code can deny instead of asking: Codex, Gemini would wait at its prompts.",
    );
  });
});
