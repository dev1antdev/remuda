import { describe, expect, it } from "vitest";
import { concerns } from "@/features/roster/concerns";
import { fmtDate } from "@/lib/time";
import { applies, canonical, defaultsOf, failingOn, fastIn, modelIn, noticeWords, retiredFor, retiringConcerns, type Found, type Retirement, type RetiringHit } from "./retired";

// FAST_48 is the server's table entry as the roster carries it (internal/retire)
const FAST_48: Retirement = { agent: "claude", model: "claude-opus-4-8", name: "Opus 4.8", mode: "fast", on: "2026-11-30", replacement: "claude-opus-5-5", replacement_name: "Opus 5.5", source: "Claude Code 2.1.316" };
const before = Date.parse("2026-11-20T12:00:00Z");
const after = Date.parse("2026-12-01T00:00:00Z");
const FAST = `--settings '{"fastMode":true}'`;
const DATE = fmtDate(new Date(2026, 10, 30)); // the day in the test's own locale

describe("reading a model the way the server does", () => {
  it("canonical drops a provider, a window, a Vertex or Bedrock tail and a date", () => {
    expect(canonical(" Claude-Opus-4-8[1m] ")).toBe("claude-opus-4-8");
    expect(canonical("us.anthropic.claude-opus-4-8-20251101-v1:0")).toBe("claude-opus-4-8");
    expect(canonical("claude-opus-4-8@20251101")).toBe("claude-opus-4-8");
    expect(canonical("opus")).toBe("opus");
  });

  it("reads the arguments' fast mode and --model, the last word winning", () => {
    expect(fastIn(FAST)).toBe(true);
    expect(fastIn(`${FAST} --settings '{"fastMode" : false}'`)).toBe(false);
    expect(fastIn("--permission-mode plan")).toBeUndefined();
    expect(fastIn('--settings "{\\"fastMode\\": true}"')).toBe(true); // double quotes, its own escaped
    expect(modelIn('--model \\"claude-opus-4-8\\"')).toBe("claude-opus-4-8");
    expect(modelIn("--model claude-opus-4-8 --verbose")).toBe("claude-opus-4-8");
    expect(modelIn("--verbose")).toBe("");
  });

  it("applies to the model in its mode alone, for its agent", () => {
    expect(applies(FAST_48, "", "claude-opus-4-8", true)).toBe(true);
    expect(applies(FAST_48, "claude", "claude-opus-4-8", false)).toBe(false);
    expect(applies(FAST_48, "claude", "opus", true)).toBe(false);
    expect(applies(FAST_48, "codex", "claude-opus-4-8", true)).toBe(false);
    expect(applies({ ...FAST_48, mode: undefined }, "claude", "claude-opus-4-8", false)).toBe(true);
  });

  it("fails from the start of its day in UTC", () => {
    expect(failingOn(FAST_48, Date.parse("2026-11-29T23:59:00Z"))).toBe(false);
    expect(failingOn(FAST_48, Date.parse("2026-11-30T00:00:00Z"))).toBe(true);
  });
});

describe("retiredFor", () => {
  const defaults = defaultsOf([
    { id: "gog", health: { claude: "2.1.322", claude_model: "claude-opus-4-8", claude_fast: true } },
    { id: "pi", health: { claude: "2.1.322" } },
    { id: "bare", health: {} },
  ]);

  it("knows only the machines with Claude Code", () => {
    expect(Object.keys(defaults)).toEqual(["gog", "pi"]);
  });

  it("finds a setting on its own machine, the field and the arguments first", () => {
    expect(retiredFor([FAST_48], { agent: "claude", model: "claude-opus-4-8", args: FAST, machine: "pi" }, defaults)).toEqual({ retirement: FAST_48, model: "field", fast: "arguments", machines: ["pi"] });
    expect(retiredFor([FAST_48], { agent: "claude", model: "claude-opus-4-8", args: "", machine: "pi" }, defaults)).toBeNull();
    expect(retiredFor([FAST_48], { agent: "claude", model: "", args: "", machine: "gog" }, defaults)).toEqual({ retirement: FAST_48, model: "default", fast: "default", machines: ["gog"] });
    expect(retiredFor([FAST_48], { agent: "claude", model: "claude-opus-5-5", args: "", machine: "gog" }, defaults)).toBeNull();
    expect(retiredFor([FAST_48], { agent: "claude", model: "claude-opus-5-5", args: `--model claude-opus-4-8 ${FAST}`, machine: "pi" }, defaults)?.model).toBe("arguments");
  });

  it("weighs a setting for any machine on each one, or on its own words where none is known", () => {
    expect(retiredFor([FAST_48], { agent: "claude", model: "", args: "", machine: "" }, defaults)?.machines).toEqual(["gog"]);
    expect(retiredFor([FAST_48], { agent: "claude", model: "claude-opus-4-8", args: FAST, machine: "" }, defaults)?.machines).toEqual(["gog", "pi"]);
    expect(retiredFor([FAST_48], { agent: "claude", model: "claude-opus-4-8", args: FAST, machine: "" }, {})).toEqual({ retirement: FAST_48, model: "field", fast: "arguments", machines: [] });
    expect(retiredFor([], { agent: "claude", model: "claude-opus-4-8", args: FAST, machine: "" }, {})).toBeNull();
  });
});

describe("the words", () => {
  it("a form's notice says when, what to use, and why the machine's settings make it so", () => {
    const own = { retirement: FAST_48, model: "field", fast: "arguments", machines: ["pi"] } satisfies Found;
    expect(noticeWords(own, before)).toBe(`Fast mode on Opus 4.8 ends on ${DATE}: after that its requests fail and do not fall back. Use Opus 5.5.`);
    expect(noticeWords(own, before, [], true), "the form's button says what to use").toBe(`Fast mode on Opus 4.8 ends on ${DATE}: after that its requests fail and do not fall back.`);
    expect(noticeWords(own, after)).toBe(`Fast mode on Opus 4.8 was removed on ${DATE}: its requests fail now and do not fall back. Use Opus 5.5.`);
    const settled = { retirement: FAST_48, model: "default", fast: "default", machines: ["gog"] } satisfies Found;
    expect(noticeWords(settled, before, ["GOG"])).toBe(
      `Fast mode on Opus 4.8 ends on ${DATE}: after that its requests fail and do not fall back. Opus 4.8 is the default model in Claude Code's settings on GOG. Fast mode is saved on in Claude Code's settings on GOG. Use Opus 5.5.`,
    );
  });

  // hit is what the roster carries: the retirement by its key alone, the table says the rest
  const hit = (kind: RetiringHit["kind"], id: string): RetiringHit => ({ agent: "claude", model: "claude-opus-4-8", mode: "fast", kind, id, machine: "gog", failing: false });

  it("the ⓘ counts what is set to it by kind, one concern per retirement, bad once its day came", () => {
    const hits = [hit("recipe", "r1"), hit("schedule", "s1"), hit("schedule", "s2"), hit("project", "3"), hit("stage", "3/1"), hit("pane", "w1:p2")];
    expect(retiringConcerns(hits, [FAST_48], before)).toEqual([
      {
        key: "retiring-claude-opus-4-8|fast",
        text: `Fast mode on Opus 4.8 ends on ${DATE}, set for 1 recipe, 2 agent schedules, the agents of 1 project, 1 pipeline stage and 1 running agent here: move to Opus 5.5`,
        bad: false,
      },
    ]);
    expect(retiringConcerns([hit("pane", "w1:p2"), hit("pane", "w1:p2")], [FAST_48], after)).toEqual([
      { key: "retiring-claude-opus-4-8|fast", text: `Fast mode on Opus 4.8 was removed on ${DATE} and is still set for 1 running agent here: their requests fail now — move to Opus 5.5`, bad: true },
    ]);
    expect(retiringConcerns(undefined, [FAST_48], before)).toEqual([]);
    expect(retiringConcerns([hit("pane", "w1:p2")], [], before)).toEqual([]); // a key the table no longer holds says nothing
  });

  it("stands among the machine's concerns", () => {
    const list = concerns(undefined, undefined, "v0.13.0", true, [], false, { machine: "GOG", retiring: [hit("schedule", "s1")], retirements: [FAST_48], now: before });
    expect(list.map((c) => c.key)).toEqual(["retiring-claude-opus-4-8|fast"]);
  });
});
