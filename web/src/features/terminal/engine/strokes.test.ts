import { describe, expect, it } from "vitest";
import { keyOf, MAX_TEXT, strokeOf, takeBatch, withModifier, type KeyLike, type Stroke } from "./strokes";

const press = (key: string, code = "", mods: Partial<KeyLike> = {}): KeyLike => ({ key, code, ctrlKey: false, altKey: false, shiftKey: false, metaKey: false, ...mods });

describe("keyOf", () => {
  it("names the keys a terminal needs", () => {
    expect(keyOf(press("Enter", "Enter"), false)).toBe("enter");
    expect(keyOf(press("Enter", "Enter", { shiftKey: true }), false)).toBe("shift+enter");
    expect(keyOf(press("Enter", "Enter", { altKey: true }), true)).toBe("alt+enter");
    expect(keyOf(press("Tab", "Tab", { shiftKey: true }), false)).toBe("shift+tab");
    expect(keyOf(press("Escape", "Escape"), false)).toBe("esc");
    expect(keyOf(press("Backspace", "Backspace"), false)).toBe("backspace");
    expect(keyOf(press("Delete", "Delete"), false)).toBe("delete");
    expect(keyOf(press("ArrowUp", "ArrowUp", { ctrlKey: true }), false)).toBe("ctrl+up");
    expect(keyOf(press("Home", "Home"), false)).toBe("home");
    expect(keyOf(press("PageDown", "PageDown"), false)).toBe("pagedown");
  });
  it("takes Ctrl and a letter by its place on the keyboard, whatever the layout", () => {
    expect(keyOf(press("с", "KeyC", { ctrlKey: true }), false)).toBe("ctrl+c");
    expect(keyOf(press("r", "KeyR", { ctrlKey: true }), true)).toBe("ctrl+r");
    expect(keyOf(press("[", "BracketLeft", { ctrlKey: true }), false)).toBe("esc");
    expect(keyOf(press("b", "KeyB", { altKey: true }), false)).toBe("alt+b");
  });
  it("leaves the browser's and the text's own keys alone", () => {
    expect(keyOf(press("a", "KeyA"), false)).toBe(null);
    expect(keyOf(press("A", "KeyA", { shiftKey: true }), false)).toBe(null);
    expect(keyOf(press("v", "KeyV", { ctrlKey: true }), false)).toBe(null); // the paste, off a Mac
    expect(keyOf(press("v", "KeyV", { ctrlKey: true }), true)).toBe("ctrl+v");
    expect(keyOf(press("c", "KeyC", { metaKey: true }), true)).toBe(null);
    expect(keyOf(press("F", "KeyF", { ctrlKey: true, shiftKey: true }), false)).toBe(null);
    expect(keyOf(press("ƒ", "KeyF", { altKey: true }), true)).toBe(null); // a Mac's Option types a character
    expect(keyOf(press("@", "KeyQ", { ctrlKey: true, altKey: true }), false)).toBe(null); // AltGr
    expect(keyOf(press("Process", "KeyA"), false)).toBe(null);
    expect(keyOf(press("Enter", "Enter", { isComposing: true }), false)).toBe(null);
    expect(keyOf(press("Tab", "Tab", { ctrlKey: true }), false)).toBe(null);
    expect(keyOf(press("Home", "Home", { shiftKey: true }), false)).toBe(null);
  });
});

describe("strokeOf", () => {
  it("reads the key bar's sequences as keys", () => {
    expect(strokeOf("\x1b")).toEqual({ key: "esc" });
    expect(strokeOf("\x1bOA")).toEqual({ key: "up" });
    expect(strokeOf("\x1b[D")).toEqual({ key: "left" });
    expect(strokeOf("\x1b\r")).toEqual({ key: "alt+enter" });
    expect(strokeOf("\x1b[Z")).toEqual({ key: "shift+tab" });
    expect(strokeOf("\x1b[6~")).toEqual({ key: "pagedown" });
    expect(strokeOf("\x03")).toEqual({ key: "ctrl+c" });
    expect(strokeOf("\x0c")).toEqual({ key: "ctrl+l" });
    expect(strokeOf("x")).toEqual({ text: "x" });
  });
});

describe("withModifier", () => {
  it("spends the key bar's Ctrl or Alt on the next letter or key", () => {
    expect(withModifier({ text: "c" }, "ctrl")).toEqual({ stroke: { key: "ctrl+c" }, used: true });
    expect(withModifier({ text: "B" }, "alt")).toEqual({ stroke: { key: "alt+b" }, used: true });
    expect(withModifier({ key: "left" }, "alt")).toEqual({ stroke: { key: "alt+left" }, used: true });
    expect(withModifier({ text: "ab" }, "ctrl")).toEqual({ stroke: { text: "ab" }, used: false });
    expect(withModifier({ key: "ctrl+c" }, "alt")).toEqual({ stroke: { key: "ctrl+c" }, used: false });
    expect(withModifier({ key: "home" }, "ctrl")).toEqual({ stroke: { key: "home" }, used: false });
    expect(withModifier({ text: "x" }, null)).toEqual({ stroke: { text: "x" }, used: false });
  });
});

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
