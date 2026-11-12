import { describe, expect, it } from "vitest";
import { agentSchedule, agentWords, deniedWords, SCOUT_PROMPT, watchedWords } from "./agent";
import { kindOf } from "./digest";

describe("agent schedules", () => {
  it("are of kind agent and close their tab unless told otherwise", () => {
    const s = agentSchedule({ machine: "local", dir: "/x", agent: "codex", prompt: SCOUT_PROMPT });
    expect(s.kind).toBe("agent");
    expect(s.close).toBe(true);
    expect(s.enabled).toBe(true);
    expect(s.model).toBe("");
    expect(agentSchedule({ machine: "local", dir: "/x", agent: "codex", prompt: "hi", close: false, model: "gpt-5" })).toMatchObject({ close: false, model: "gpt-5" });
    expect(kindOf(s)).toBe("agent");
    expect(kindOf({ kind: undefined })).toBe("prompt");
  });

  it("deny prompts unless told otherwise, and say so for Claude Code alone", () => {
    expect(agentSchedule({ machine: "local", dir: "/x", agent: "claude", prompt: "hi" }).deny_prompts).toBe(true);
    expect(agentSchedule({ machine: "local", dir: "/x", agent: "claude", prompt: "hi", deny_prompts: false }).deny_prompts).toBe(false);
    expect(agentWords({ close: false, deny_prompts: true, agent: "claude" }, "GOG", "remuda", "Claude Code")).toBe("GOG · remuda · Claude Code · denies instead of asking");
    expect(agentWords({ close: false, deny_prompts: true, agent: "codex" }, "GOG", "remuda", "Codex")).toBe("GOG · remuda · Codex");
    expect(agentWords({ close: false, agent: "claude" }, "GOG", "remuda", "Claude Code")).toBe("GOG · remuda · Claude Code");
    expect(deniedWords({ last_denied: [{ tool: "Bash", what: "touch x" }, { tool: "Write", what: "/w/a.txt" }] })).toBe("2 calls denied: Bash · touch x; Write · /w/a.txt");
    expect(deniedWords({})).toBe("");
  });

  it("say where the agent runs and whether it goes", () => {
    expect(agentWords({ close: true }, "GOG", "remuda", "Codex")).toBe("GOG · remuda · Codex · closes when answered");
    expect(agentWords({ close: false }, "This Mac", "remuda", "Claude Code")).toBe("This Mac · remuda · Claude Code");
    expect(watchedWords({ last_pane: { machine: "local", pane: "w5:p2", tab: "w5:t2", workspace: "w5", made_workspace: false, worked: true, at: "" } })).toBe("watching w5:p2");
    expect(watchedWords({})).toBe("");
  });
});
