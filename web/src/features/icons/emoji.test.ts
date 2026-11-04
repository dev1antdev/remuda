import { describe, expect, it } from "vitest";
import { pushRecent, searchEmoji } from "./emoji";
import { EMOJI_GROUPS, type EmojiGroup } from "./emojiData";

const fixture: EmojiGroup[] = [
  { id: "animals", name: "Animals & nature", emoji: [["🦊", "fox face"], ["🐶", "dog face"], ["🌲", "evergreen tree"]] },
  { id: "objects", name: "Objects", emoji: [["🦴", "bone"], ["🔥", "fire"], ["🧯", "fire extinguisher"]] },
];

describe("searchEmoji", () => {
  it("wants every typed word to start a word of the name, case-folded, in the groups' order", () => {
    expect(searchEmoji(fixture, "FOX").map(([e]) => e)).toEqual(["🦊"]);
    expect(searchEmoji(fixture, "face").map(([e]) => e)).toEqual(["🦊", "🐶"]);
    expect(searchEmoji(fixture, "fire ext").map(([e]) => e)).toEqual(["🧯"]);
    expect(searchEmoji(fixture, "fire").map(([e]) => e)).toEqual(["🔥", "🧯"]);
    expect(searchEmoji(fixture, "ace")).toEqual([]);
    expect(searchEmoji(fixture, "  ")).toEqual([]);
  });

  it("finds an emoji typed as itself and stops at the limit", () => {
    expect(searchEmoji(fixture, "🌲")).toEqual([["🌲", "evergreen tree"]]);
    expect(searchEmoji(fixture, "f", 2).map(([e]) => e)).toEqual(["🦊", "🐶"]);
  });

  it("reads the real data", () => {
    expect(EMOJI_GROUPS.map((g) => g.id)).toEqual(["smileys", "people", "animals", "food", "travel", "activities", "objects", "symbols", "flags"]);
    for (const group of EMOJI_GROUPS) expect(group.emoji.length).toBeGreaterThan(50);
    expect(searchEmoji(EMOJI_GROUPS, "fox").map(([e]) => e)).toContain("🦊");
    expect(searchEmoji(EMOJI_GROUPS, "flag ukr").map(([e]) => e)).toContain("🇺🇦");
    const all = EMOJI_GROUPS.flatMap((g) => g.emoji.map(([e]) => e));
    expect(new Set(all).size).toBe(all.length);
  });
});

describe("pushRecent", () => {
  it("puts the pick first, once, and keeps at most max", () => {
    expect(pushRecent([], "🦊")).toEqual(["🦊"]);
    expect(pushRecent(["🐶", "🦊", "🔥"], "🦊")).toEqual(["🦊", "🐶", "🔥"]);
    expect(pushRecent(["a", "b", "c"], "d", 3)).toEqual(["d", "a", "b"]);
  });
});
