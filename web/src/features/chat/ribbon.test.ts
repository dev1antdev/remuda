import { describe, expect, it } from "vitest";
import type { Part, Turn } from "./api";
import { QUIET_MS, RIBBON_MS, ribbon, stepFrom, stretchAt, stretchWords, toneOf, turnWords, type RibbonModel } from "./ribbon";
import { COMPACTION_PREFIX } from "./timeline";

const NOW = Date.parse("2026-11-13T12:00:00.000Z");
const min = (m: number) => m * 60_000;
// at is a moment `m` minutes before now
const at = (m: number) => new Date(NOW - min(m)).toISOString();
const user = (m: number, text = "go", id = `u${m}`): Turn => ({ role: "user", ts: at(m), ids: [id], parts: [{ type: "text", text }] });
const answer = (m: number, parts: Part[], took?: number): Turn => ({ role: "assistant", ts: at(m), ids: [`a${m}`], parts, ...(took ? { took } : {}) });
const call = (name: string, m: number, seconds: number | null, input: unknown = {}, extra: Partial<Part> = {}): Part => ({
  type: "tool",
  tool_name: name,
  tool_id: `t-${name}-${m}`,
  at: at(m),
  input,
  ...(seconds === null ? {} : { output: "ok", ...(seconds > 0 ? { took: seconds } : {}) }),
  ...extra,
});
const text = (m: number, words = "ok"): Part => ({ type: "text", text: words, at: at(m) });
const tones = (model: RibbonModel) => model.stretches.map((s) => s.tone);
const span = (model: RibbonModel, tone: string) => model.stretches.filter((s) => s.tone === tone).map((s) => [(NOW - s.from) / 60_000, (NOW - s.to) / 60_000]);

describe("ribbon", () => {
  it("colours each call by its kind, and the gaps between them as thought", () => {
    const turns = [
      user(20),
      answer(19.9, [call("Read", 19.9, 6, { file_path: "/w/app/src/app.go" }), call("Edit", 19, 12, { file_path: "/w/app/src/app.go" }), call("Bash", 18, 48, { command: "go test ./..." }), call("TodoWrite", 17, 0), text(16)], 4 * 60),
    ];
    const model = ribbon(turns, { now: NOW, live: null, cut: false });
    expect(model.from).toBe(NOW - RIBBON_MS);
    expect(model.stretches.filter((s) => s.call).map((s) => [s.tone, s.call!.name])).toEqual([
      ["read", "Read"],
      ["write", "Edit"],
      ["shell", "Bash"],
      ["tool", "TodoWrite"],
    ]);
    expect(tones(model).filter((t) => t === "think").length).toBeGreaterThan(0);
    expect(tones(model)).not.toContain("quiet");
    expect(model.turn).toEqual({ took: 240, tools: 4, failed: 0, live: false });
    expect(turnWords(model.turn)).toBe("4 min, 4 tools");
  });

  it("calls a gap quiet once three minutes pass with no call running and nothing written", () => {
    const turns = [user(20), answer(19.9, [call("Bash", 19, 30, { command: "make" }), text(16), call("Read", 10, 1, { file_path: "/x" })], 10 * 60)];
    const model = ribbon(turns, { now: NOW, live: null, cut: false });
    // the call ends at 18.5; the text at 16 breaks the gap; 16 → 13 is thought, 13 → 10 quiet
    expect(span(model, "quiet")).toEqual([[13, 10]]);
    expect(span(model, "think")).toContainEqual([18.5, 13]); // one stretch: a text written in it keeps it going
  });

  it("runs to now while the agent works, the call still open drawn as running", () => {
    const turns = [user(5), answer(4.9, [call("Bash", 1, null, { command: "go test ./..." })])];
    const model = ribbon(turns, { now: NOW, live: "run", cut: false });
    const open = model.stretches.find((s) => s.call)!;
    expect(open).toMatchObject({ tone: "shell", to: NOW });
    expect(open.call!.running).toBe(true);
    expect(model.turn!.live).toBe(true);
    expect(turnWords(model.turn)).toBe("5 min so far, 1 tool");
    expect(stretchWords(open, NOW, "/w", "")).toBe("Bash · go test ./... · 1 min so far");
  });

  it("goes quiet at the end of a turn that wrote nothing for three minutes while the agent still works", () => {
    const model = ribbon([user(10), answer(9.9, [text(9.9)])], { now: NOW, live: "run", cut: false });
    expect(model.stretches.at(-1)).toMatchObject({ tone: "quiet", to: NOW });
    expect((model.stretches.at(-1)!.from - (NOW - min(9.9))) / 1).toBe(QUIET_MS);
  });

  it("waits while the agent asks: the open call, or the time after its last record", () => {
    const blocked = ribbon([user(3), answer(2.9, [call("Bash", 2, null, { command: "rm -rf build" })])], { now: NOW, live: "input", cut: false });
    expect(blocked.stretches.find((s) => s.call)!.tone).toBe("wait");
    const dialog = ribbon([user(3), answer(2.9, [text(2.5)])], { now: NOW, live: "input", cut: false });
    expect(dialog.stretches.at(-1)).toMatchObject({ tone: "wait", to: NOW });
    expect(toneOf(call("AskUserQuestion", 1, 40), 40_000)).toBe("wait");
    expect(toneOf(call("Agent", 1, 300), 300_000)).toBe("wait");
    expect(toneOf(call("Agent", 1, 2), 2_000)).toBe("tool");
    const denied: Part = { ...call("Write", 1, 0), is_error: true, output: "Permission for this tool use was denied. It requires approval, and this session has no approval surface." };
    expect(toneOf(denied, 0)).toBe("wait");
    expect(toneOf({ ...call("Bash", 1, 3), is_error: true, output: "exit status 1" }, 3000)).toBe("shell");
  });

  it("leaves what came before the window out, and what came before the turns read as not read", () => {
    const old = [user(50), answer(49, [call("Read", 49, 5)], 60)];
    expect(ribbon(old, { now: NOW, live: null, cut: false }).stretches).toEqual([]);
    const tail = [answer(12, [call("Edit", 12, 3)], 900)];
    const cut = ribbon(tail, { now: NOW, live: null, cut: true });
    expect(cut.stretches[0]).toMatchObject({ tone: "unread", from: NOW - RIBBON_MS, to: NOW - min(12) });
    expect(ribbon(tail, { now: NOW, live: null, cut: false }).stretches.some((s) => s.tone === "unread")).toBe(false);
    const compacted = [user(10, `${COMPACTION_PREFIX}. Summary`), answer(9.9, [text(9.8)], 20)];
    expect(ribbon(compacted, { now: NOW, live: null, cut: false }).stretches[0]!.tone).toBe("unread");
  });

  it("finds the stretch under a press: a narrow call within a few pixels, over the gap it lies in", () => {
    const model = ribbon([user(20), answer(19.9, [text(13), call("Edit", 10, 1, { file_path: "/w/app/a.go" })], 15 * 60)], { now: NOW, live: null, cut: false });
    const share = (m: number) => (RIBBON_MS - min(m)) / RIBBON_MS;
    expect(stretchAt(model, share(10), 300)!.call!.name).toBe("Edit");
    expect(stretchAt(model, share(10) + 3 / 300, 300)!.call!.name).toBe("Edit");
    expect(stretchAt(model, share(12), 300)!.tone).toBe("think");
    expect(stretchAt(model, share(29), 300)).toBeNull();
    const edit = stretchAt(model, share(10), 300)!;
    expect(stretchWords(edit, NOW, "/w/app", "")).toBe("Edit · a.go · 1.0 s · 10 min ago");
  });

  it("walks the stretches in time with the keys", () => {
    const model = ribbon([user(20), answer(19.9, [call("Read", 19, 2), call("Edit", 18, 2)], 120)], { now: NOW, live: null, cut: false });
    const first = stepFrom(model, null, 1)!;
    expect(first.tone).toBe("think");
    const next = stepFrom(model, first.key, 1)!;
    expect(next.call?.name).toBe("Read");
    expect(stepFrom(model, null, -1)!.from).toBeGreaterThanOrEqual(next.from);
  });

  it("says what the last turn came to, failures counted, nothing when there is none", () => {
    const model = ribbon([user(9), answer(8.9, [call("Bash", 8.8, 4, {}, { is_error: true }), call("Bash", 8.5, 4)], 67)], { now: NOW, live: null, cut: false });
    expect(turnWords(model.turn)).toBe("1 min 7 s, 2 tools, 1 failed");
    expect(turnWords(null)).toBe("Nothing in the last 30 minutes");
    expect(turnWords({ took: 12, tools: 0, failed: 0, live: true })).toBe("12 s so far, no tools");
  });
});
