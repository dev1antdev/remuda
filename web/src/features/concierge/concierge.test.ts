import { describe, expect, it } from "vitest";
import type { ChatResponse, Turn } from "@/features/chat/api";
import type { Machine, Pane, Session } from "@/features/roster/api";
import { setLang } from "@/lib/i18n";
import { comingUp, conciergePane, HINTS, labelOf, machineChoices, needsStart, shownTurns, splitConcierge } from "./concierge";

const pane = (id: string, agent = "claude", focused = false): Pane => ({ id, title: id, agent, cwd: "/state/concierge", focused, status: "idle", cols: 80, rows: 24 });
const session = (id: string, panes: Pane[][], extra: Partial<Session> = {}): Session => ({
  id,
  name: id,
  status: "idle",
  active_tab_id: `${id}:t${panes.length}`,
  tabs: panes.map((list, i) => ({ id: `${id}:t${i + 1}`, label: String(i + 1), number: i + 1, named: false, status: "idle", panes: list })),
  ...extra,
});
const machine = (id: string, sessions: Session[]): Machine => ({ id, label: id, state: "connected", sessions });

describe("splitConcierge", () => {
  it("takes the concierge's workspace out of every machine and hands it to its place alone", () => {
    const api = session("w1", [[pane("w1:p1")]]);
    const desk = session("w9", [[pane("w9:p1")]], { concierge: true, dir: "/state/concierge" });
    const { machines, concierges } = splitConcierge([machine("mac", [api, desk]), machine("gog", [session("w2", [[pane("w2:p1", "codex")]])])]);
    expect(machines.map((m) => m.sessions.map((s) => s.id))).toEqual([["w1"], ["w2"]]);
    expect(concierges).toEqual([{ machineId: "mac", session: desk }]);
  });
  it("leaves a roster without one as it is, so nothing redraws for it", () => {
    const fleet = [machine("mac", [session("w1", [[pane("w1:p1")]])])];
    const split = splitConcierge(fleet);
    expect(split.machines).toBe(fleet);
    expect(split.concierges).toEqual([]);
  });
  it("lists one lingering on another machine too, for its place to tell apart", () => {
    const { concierges } = splitConcierge([machine("mac", [session("w9", [[pane("w9:p1")]], { concierge: true })]), machine("gog", [session("w4", [[pane("w4:p1")]], { concierge: true })])]);
    expect(concierges.map((c) => c.machineId)).toEqual(["mac", "gog"]);
  });
});

describe("conciergePane", () => {
  const desk = session("w9", [[pane("w9:p1")], [pane("w9:p2"), pane("w9:p3", "claude", true)]], { concierge: true });
  const concierges = [{ machineId: "mac", session: desk }];
  it("is the pane a start named, else the active tab's focused one", () => {
    expect(conciergePane(concierges, "mac", "w9:p1")?.id).toBe("w9:p1");
    expect(conciergePane(concierges, "mac")?.id).toBe("w9:p3");
    expect(conciergePane(concierges, "mac", "w1:p1")?.id).toBe("w9:p3");
  });
  it("is none on a machine where it does not stand", () => {
    expect(conciergePane(concierges, "gog")).toBeUndefined();
  });
});

describe("needsStart", () => {
  it("starts it when nothing stands or its pane is a bare shell, and waits for a pane not listed yet", () => {
    expect(needsStart(false, undefined)).toBe(true);
    expect(needsStart(true, pane("w9:p1", ""))).toBe(true);
    expect(needsStart(true, pane("w9:p1"))).toBe(false);
    expect(needsStart(true, undefined)).toBe(false);
  });
});

describe("shownTurns", () => {
  const turn = { role: "assistant", parts: [] } as unknown as Turn;
  const chat = (id: string): ChatResponse => ({ session_id: id, sessions: [], turns: [turn] });
  it("draws nothing of the conversation a New conversation set aside", () => {
    expect(shownTurns(chat("old"), "old")).toEqual([]);
    expect(shownTurns(chat("new"), "old")).toEqual([turn]);
    expect(shownTurns(chat("old"), null)).toEqual([turn]);
    expect(shownTurns(null, null)).toEqual([]);
  });
});

describe("the words", () => {
  it("says what comes up while it is not ready", () => {
    expect(comingUp("GOG", undefined)).toBe("Starting the concierge on GOG…");
    expect(comingUp("GOG", pane("w9:p1", ""))).toBe("Starting Claude Code on GOG…");
    expect(comingUp("GOG", pane("w9:p1"))).toBe("");
  });
  it("lists the picker's machines, an away one saying so", () => {
    const view = { machine: "mac", machines: [{ id: "mac", label: "This Mac", connected: true, mac: true }, { id: "lap", label: "Laptop", connected: false }] };
    expect(machineChoices(view)).toEqual([
      { value: "mac", label: "This Mac", hint: undefined },
      { value: "lap", label: "Laptop", hint: "away" },
    ]);
    expect(labelOf(view, "lap")).toBe("Laptop");
    expect(labelOf(view, "gone")).toBe("gone");
  });
  it("offers three questions, in Russian too", () => {
    expect(HINTS).toHaveLength(3);
    setLang("ru");
    expect(comingUp("GOG", undefined)).toBe("Запускаем консьержа на GOG…");
    setLang("en");
  });
});
