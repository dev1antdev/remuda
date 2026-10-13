import { describe, expect, it } from "vitest";
import { applyModifier, bracketed } from "./keys";

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

describe("bracketed", () => {
  it("wraps a paste the way xterm would, line breaks as CR", () => {
    expect(bracketed("a\r\nb\nc")).toBe("\x1b[200~a\rb\rc\x1b[201~");
  });
});
