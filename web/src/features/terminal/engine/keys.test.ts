import { describe, expect, it } from "vitest";
import { applyModifier, bracketed, isFindKey } from "./keys";

describe("applyModifier", () => {
  it("leaves everything alone without a modifier", () => {
    expect(applyModifier(null, "a")).toBe("a");
    expect(applyModifier(null, "\x1b[A")).toBe("\x1b[A");
  });
  it("encodes a printable character under Ctrl or Alt", () => {
    expect(applyModifier("ctrl", "a")).toBe("\x01");
    expect(applyModifier("ctrl", "C")).toBe("\x03");
    expect(applyModifier("alt", "a")).toBe("\x1ba");
  });
  it("encodes an arrow as the modified CSI form, whatever the cursor mode", () => {
    expect(applyModifier("ctrl", "\x1b[A")).toBe("\x1b[1;5A");
    expect(applyModifier("alt", "\x1bOB")).toBe("\x1b[1;3B");
  });
  it("passes anything else through", () => {
    expect(applyModifier("ctrl", "\r")).toBe("\r");
    expect(applyModifier("alt", "ab")).toBe("ab");
  });
});

describe("isFindKey", () => {
  const key = (over: Partial<{ code: string; metaKey: boolean; ctrlKey: boolean; shiftKey: boolean; altKey: boolean }>) => ({ code: "KeyF", metaKey: false, ctrlKey: false, shiftKey: false, altKey: false, ...over });
  it("is ⌘F on a Mac, never with Shift or Alt", () => {
    expect(isFindKey(key({ metaKey: true }), true)).toBe(true);
    expect(isFindKey(key({ metaKey: true, shiftKey: true }), true)).toBe(false);
    expect(isFindKey(key({ ctrlKey: true, shiftKey: true }), true)).toBe(false);
  });
  it("is Ctrl+Shift+F elsewhere, leaving Ctrl+F to the shell", () => {
    expect(isFindKey(key({ ctrlKey: true, shiftKey: true }), false)).toBe(true);
    expect(isFindKey(key({ ctrlKey: true }), false)).toBe(false);
    expect(isFindKey(key({ ctrlKey: true, shiftKey: true, altKey: true }), false)).toBe(false);
    expect(isFindKey(key({ code: "KeyG", ctrlKey: true, shiftKey: true }), false)).toBe(false);
  });
});

describe("bracketed", () => {
  it("wraps a paste the way xterm would, line breaks as CR", () => {
    expect(bracketed("a\r\nb\nc")).toBe("\x1b[200~a\rb\rc\x1b[201~");
  });
});

describe("a one-shot Ctrl is xterm's table", () => {
  it("folds a letter and keeps the seven characters of the table", () => {
    expect(applyModifier("ctrl", "c")).toBe("\x03");
    expect(applyModifier("ctrl", "@")).toBe("\x00");
    expect(applyModifier("ctrl", "?")).toBe("\x7f");
  });
  it("passes anything else through: Ctrl+- is not Enter, Ctrl+3 not XOFF", () => {
    for (const key of ["-", "$", "3", "1", " ", ".", "/"]) expect(applyModifier("ctrl", key)).toBe(key);
  });
});
