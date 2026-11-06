import { describe, expect, it } from "vitest";
import { floatKeyTakes, floatMachine, isFloatKey, keyWords, shellView } from "./float";

const press = (over: Partial<Parameters<typeof isFloatKey>[0]>) => ({ code: "Backquote", ctrlKey: true, metaKey: false, altKey: false, shiftKey: false, ...over });

describe("isFloatKey", () => {
  it("is Ctrl and the backquote key alone, by key code", () => {
    expect(isFloatKey(press({}))).toBe(true);
    expect(isFloatKey(press({ ctrlKey: false }))).toBe(false);
    expect(isFloatKey(press({ metaKey: true }))).toBe(false);
    expect(isFloatKey(press({ altKey: true }))).toBe(false);
    expect(isFloatKey(press({ shiftKey: true }))).toBe(false);
    expect(isFloatKey(press({ code: "KeyK" }))).toBe(false);
  });
});

describe("floatMachine", () => {
  const machines = [
    { id: "local", state: "connected" },
    { id: "gog", state: "connected" },
    { id: "roofr", state: "error" },
  ];

  it("takes the one asked while connected, else the pane's, else this machine", () => {
    expect(floatMachine("gog", "local", machines)).toBe("gog");
    expect(floatMachine("roofr", "gog", machines)).toBe("gog");
    expect(floatMachine(null, "roofr", machines)).toBe("local");
    expect(floatMachine(null, null, machines)).toBe("local");
  });

  it("falls back to any connected machine, and to none", () => {
    expect(floatMachine(null, null, [{ id: "local", state: "error" }, { id: "gog", state: "connected" }])).toBe("gog");
    expect(floatMachine("gog", "local", [{ id: "local", state: "connecting" }])).toBeNull();
  });
});

describe("floatKeyTakes", () => {
  it("takes the key only while nothing is open over the page", () => {
    expect(floatKeyTakes(press({}), false)).toBe(true);
    expect(floatKeyTakes(press({}), true)).toBe(false);
    expect(floatKeyTakes(press({ ctrlKey: false }), false)).toBe(false);
  });
});

describe("shellView", () => {
  it("opens, then shows the shell, even before the roster lists it", () => {
    expect(shellView("", false, false, true)).toBe("opening");
    expect(shellView("w3:p1", false, false, true)).toBe("live");
    expect(shellView("w3:p1", true, true, true)).toBe("live");
  });

  it("calls the shell ended only once the roster listed it and lists it no more", () => {
    expect(shellView("w3:p1", false, true, true)).toBe("ended");
  });

  it("says the machine is away rather than the shell ended while it does not answer", () => {
    expect(shellView("w3:p1", false, true, false)).toBe("away");
    expect(shellView("", false, false, false)).toBe("away");
  });
});

describe("keyWords", () => {
  it("writes the key the way each system does", () => {
    expect(keyWords(true)).toBe("⌃ `");
    expect(keyWords(false)).toBe("Ctrl `");
  });
});
