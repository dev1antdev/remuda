import { describe, expect, it } from "vitest";
import type { Machine, Pane } from "./api";
import { TRINKETS, hatOf, outfits, seedOf, smallModel, speciesOf, wearersOf } from "./zooWear";

function machine(id: string, panes: Pane[]): Machine {
  return { id, label: id, state: "connected", sessions: [{ id: "w1", name: "w1", status: "idle", active_tab_id: "t1", tabs: [{ id: "t1", label: "1", number: 1, named: false, status: "idle", panes }] }] };
}
const agent = (id: string, kind: string): Pane => ({ id, title: id, agent: kind, cwd: "/w", focused: false, status: "idle", cols: 80, rows: 24 });

describe("speciesOf", () => {
  it("gives each vendor a body of its own and the rest a common one", () => {
    expect(["claude", "codex", "gemini", "kimi"].map(speciesOf)).toEqual(["claude", "codex", "gemini", "kimi"]);
    expect(speciesOf("opencode")).toBe("other");
    expect(speciesOf("")).toBe("other");
  });
});

describe("hatOf", () => {
  it("dresses each model family as the transcripts spell it", () => {
    expect(hatOf("claude-opus-5-5")).toBe("tophat");
    expect(hatOf("claude-fable-5-2")).toBe("wizard");
    expect(hatOf("us.anthropic.claude-sonnet-5-5")).toBe("beret");
    expect(hatOf("claude-haiku-5-5")).toBe("sprout");
    expect(hatOf("claude-mythos-5")).toBe("laurel");
    expect(hatOf("gpt-6.1-sol")).toBe("cap");
    expect(hatOf("openai/gpt-6.1-terra")).toBe("cap");
    expect(hatOf("o4-mini")).toBe("cap");
  });

  it("puts no hat on a model it does not know, or none", () => {
    expect(hatOf("gemini-3-pro")).toBe("");
    expect(hatOf(undefined)).toBe("");
    expect(hatOf("<synthetic>")).toBe("");
  });
});

describe("smallModel", () => {
  it("knows the little ones without taking Gemini for a mini", () => {
    expect(smallModel("claude-haiku-5-5")).toBe(true);
    expect(smallModel("gpt-6.1-mini")).toBe(true);
    expect(smallModel("gemini-3-flash-lite")).toBe(true);
    expect(smallModel("gemini-3-pro")).toBe(false);
    expect(smallModel("claude-opus-5-5")).toBe(false);
  });
});

describe("outfits", () => {
  it("is the same for the same pane on every page", () => {
    expect(seedOf("gog|p1")).toBe(seedOf("gog|p1"));
    expect(seedOf("gog|p1")).not.toBe(seedOf("gog|p2"));
    const once = outfits([{ key: "gog|p1", agent: "claude", model: "claude-opus-5-5" }]).get("gog|p1");
    const again = outfits([{ key: "gog|p1", agent: "claude", model: "claude-opus-5-5" }]).get("gog|p1");
    expect(again).toEqual(once);
    expect(once).toMatchObject({ species: "claude", hat: "tophat", small: false });
  });

  it("never gives two creatures of a species the same trinket while there are trinkets left", () => {
    const wearers = Array.from({ length: TRINKETS.length }, (_, i) => ({ key: `m|p${i}`, agent: "claude" }));
    const worn = outfits(wearers);
    expect(new Set([...worn.values()].map((o) => o.trinket)).size).toBe(TRINKETS.length);
  });

  it("lets creatures of different species wear the same trinket", () => {
    const worn = outfits([
      { key: "a", agent: "claude" },
      { key: "a2", agent: "codex" },
    ]);
    const codexAlone = outfits([{ key: "a2", agent: "codex" }]);
    expect(worn.get("a2")?.trinket).toBe(codexAlone.get("a2")?.trinket);
  });

  it("lists every agent of the roster in its order, shells left out, whatever the wall shows", () => {
    const roster = [machine("mac", [agent("p1", "claude"), agent("p2", ""), { ...agent("p3", "codex"), usage: { model: "gpt-5.5", name: "GPT-5.5", context: 0, window: 0, cost: 0, turns: 0, tokens: { input: 0, output: 0, cache_read: 0, cache_write: 0 } } }]), machine("gog", [agent("p1", "claude")])];
    expect(wearersOf(roster)).toEqual([
      { key: "mac|p1", agent: "claude", model: undefined },
      { key: "mac|p3", agent: "codex", model: "gpt-5.5" },
      { key: "gog|p1", agent: "claude", model: undefined },
    ]);
  });

  it("gives a creature the same trinket however few of the others the wall shows, once dressed over the roster", () => {
    // two Claude Codes whose seeds name the same trinket: the later one wears the next, and keeps it with the first hidden
    const keys = Array.from({ length: 200 }, (_, i) => `m|p${i}`);
    const first = keys[0]!;
    const twin = keys.find((key) => key !== first && seedOf(key) % TRINKETS.length === seedOf(first) % TRINKETS.length)!;
    const roster = [machine("m", [agent(first.slice(2), "claude"), agent(twin.slice(2), "claude")])];
    const dressed = outfits(wearersOf(roster));
    expect(dressed.get(twin)?.trinket).not.toBe(dressed.get(first)?.trinket);
    expect(outfits([{ key: twin, agent: "claude" }]).get(twin)?.trinket).toBe(dressed.get(first)?.trinket); // dressed alone it would take the first one's
  });

  it("keeps an earlier creature's trinket when another joins the wall after it", () => {
    const before = outfits([{ key: "m|p1", agent: "claude" }]).get("m|p1")?.trinket;
    const after = outfits([
      { key: "m|p1", agent: "claude" },
      { key: "m|p9", agent: "claude" },
    ]).get("m|p1")?.trinket;
    expect(after).toBe(before);
  });
});
