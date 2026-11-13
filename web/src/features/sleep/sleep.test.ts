import { describe, expect, it } from "vitest";
import type { Pane, Session, StoppedChat } from "@/features/roster/api";
import { asleepTip, busyOf, canSleep, sleepAsk, sleptWords, wakeKey, wokeWords } from "./sleep";
import { chatName, chatWhere, commandLine, someLabel, stillPicked, toggled } from "./resume";

const pane = (over: Partial<Pane>): Pane => ({ id: "w1:p1", title: "", agent: "", cwd: "/work/api", focused: false, status: "shell", cols: 80, rows: 24, ...over });
const session = (panes: Pane[], over: Partial<Session> = {}): Session => ({ id: "w1", name: "api", status: "idle", active_tab_id: "w1:t1", tabs: [{ id: "w1:t1", label: "1", number: 1, named: false, status: "idle", panes }], ...over });

describe("a sleep", () => {
  it("has something to do only where an agent runs", () => {
    expect(canSleep(session([pane({})]))).toBe(false);
    expect(canSleep(session([pane({}), pane({ id: "w1:p2", agent: "claude", status: "idle" })]))).toBe(true);
  });

  it("interrupts the agents at work or asking, and says so in Orca's words", () => {
    const s = session([
      pane({ id: "w1:p1", agent: "claude", status: "run", radar: { state: "working" } }),
      pane({ id: "w1:p2", agent: "codex", status: "input", radar: { state: "blocked" } }),
      pane({ id: "w1:p3", agent: "gemini", status: "idle", radar: { state: "idle" } }),
      pane({ id: "w1:p4", agent: "kimi", status: "run" }),
    ]);
    const busy = busyOf(s);
    expect(busy.map((b) => `${b.pane} ${b.status}`)).toEqual(["w1:p1 working", "w1:p2 blocked", "w1:p4 working"]);
    expect(sleepAsk("api", busy.slice(0, 2))).toBe("Sleep api? Still busy: Claude Code is working, Codex is waiting for your answer. Sleeping interrupts them. Waking resumes their conversations where they stopped.");
  });

  it("says what it did and what the card means", () => {
    expect(sleptWords("api", 3)).toBe("api is asleep · 3 agents stopped. Wake it from its menu.");
    expect(sleptWords("api", 1)).toBe("api is asleep · 1 agent stopped. Wake it from its menu.");
    expect(wokeWords("api", 2)).toBe("api is awake · 2 agents resumed");
    expect(asleepTip(3)).toBe("Asleep · 3 agents stopped · Wake from the menu");
  });

  it("wakes on opening once per sleep", () => {
    expect(wakeKey("gog", session([], { asleep: { at: "2026-11-13T09:00:00Z", agents: 2 } }))).toBe("gog|w1|2026-11-13T09:00:00Z");
    expect(wakeKey("gog", session([]))).toBe("");
  });
});

describe("resume after restart", () => {
  const chat = (over: Partial<StoppedChat>): StoppedChat => ({ pane: "w1:p1", agent: "claude", title: "api", dir: "/work/api", command: "claude --resume 9b1c2d3e", ...over });

  it("shows the exact command, in its folder", () => {
    expect(commandLine(chat({}))).toBe("cd /work/api && claude --resume 9b1c2d3e");
    expect(commandLine(chat({ dir: "/work/my app" }))).toBe("cd '/work/my app' && claude --resume 9b1c2d3e");
    expect(commandLine(chat({}), true)).toBe("claude --resume 9b1c2d3e");
    expect(commandLine(chat({ command: "", dir: "" }))).toBe("claude --resume");
  });

  it("names the agent and where it lived", () => {
    expect(chatName(chat({ agent: "codex" }))).toBe("Codex");
    expect(chatWhere(chat({}), "GOG")).toBe("api · GOG");
    expect(chatWhere(chat({ title: "" }), "")).toBe("w1:p1");
  });

  it("ticks, unticks and drops a chat resumed meanwhile", () => {
    expect(toggled(["w1:p1"], "w1:p2")).toEqual(["w1:p1", "w1:p2"]);
    expect(toggled(["w1:p1", "w1:p2"], "w1:p1")).toEqual(["w1:p2"]);
    expect(stillPicked(["w1:p1", "w1:p9"], [chat({})])).toEqual(["w1:p1"]);
  });

  it("offers Resume N only for some, never for none or every one", () => {
    expect(someLabel(2, 3)).toBe("Resume 2");
    expect(someLabel(0, 3)).toBeNull();
    expect(someLabel(3, 3)).toBeNull();
  });
});
