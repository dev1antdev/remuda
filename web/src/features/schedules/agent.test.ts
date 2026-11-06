import { describe, expect, it } from "vitest";
import { agentSchedule, agentWords, SCOUT_PROMPT, watchedWords } from "./agent";
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

  it("say where the agent runs and whether it goes", () => {
    expect(agentWords({ close: true }, "GOG", "remuda", "Codex")).toBe("GOG · remuda · Codex · closes when answered");
    expect(agentWords({ close: false }, "This Mac", "remuda", "Claude Code")).toBe("This Mac · remuda · Claude Code");
    expect(watchedWords({ last_pane: { machine: "local", pane: "w5:p2", tab: "w5:t2", workspace: "w5", made_workspace: false, worked: true, at: "" } })).toBe("watching w5:p2");
    expect(watchedWords({})).toBe("");
  });
});
