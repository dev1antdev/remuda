import { describe, expect, it } from "vitest";
import { MAX_TEXT, takeBatch, type Stroke } from "./strokes";

describe("takeBatch", () => {
  const all = (queue: Stroke[]) => {
    const out = [];
    for (let next = takeBatch(queue); next; next = takeBatch(next.rest)) out.push(next.batch);
    return out;
  };
  it("joins what was typed, then the keys after it, in order", () => {
    expect(all([{ text: "l" }, { text: "s" }, { key: "enter" }, { text: "p" }, { text: "wd" }, { key: "enter" }])).toEqual([
      { text: "ls", paste: false, keys: ["enter"] },
      { text: "pwd", paste: false, keys: ["enter"] },
    ]);
    expect(all([{ key: "up" }, { key: "up" }, { text: "x" }])).toEqual([
      { text: "", paste: false, keys: ["up", "up"] },
      { text: "x", paste: false, keys: [] },
    ]);
  });
  it("keeps a paste on its own, the keys after it with it", () => {
    expect(all([{ text: "a" }, { text: "one\ntwo", paste: true }, { key: "enter" }, { text: "b" }])).toEqual([
      { text: "a", paste: false, keys: [] },
      { text: "one\ntwo", paste: true, keys: ["enter"] },
      { text: "b", paste: false, keys: [] },
    ]);
  });
  it("cuts a long run of typing, never a paste", () => {
    const typed = all([{ text: "x".repeat(MAX_TEXT) }, { text: "y" }]);
    expect(typed.map((b) => b.text.length)).toEqual([MAX_TEXT, 1]);
    expect(all([{ text: "z".repeat(MAX_TEXT * 3), paste: true }])[0]!.text.length).toBe(MAX_TEXT * 3);
  });
  it("is nothing for an empty queue", () => {
    expect(takeBatch([])).toBe(null);
  });
});
