import { describe, expect, it } from "vitest";
import { splitTracking } from "./modes";

describe("splitTracking", () => {
  it("splits a DECSET that mixes a tracking mode with others, the others first", () => {
    expect(splitTracking("\x1b[?1000;2004h")).toBe("\x1b[?2004h\x1b[?1000h");
    expect(splitTracking("\x1b[?1006;1002;25l")).toBe("\x1b[?1006;25l\x1b[?1002l");
  });
  it("leaves a sequence alone when it is all ours, none of ours, or a single mode", () => {
    expect(splitTracking("\x1b[?1000;1002h")).toBe("\x1b[?1000;1002h");
    expect(splitTracking("\x1b[?25;2004h")).toBe("\x1b[?25;2004h");
    expect(splitTracking("\x1b[?1000h")).toBe("\x1b[?1000h");
  });
  it("touches nothing else in the chunk", () => {
    const chunk = "hello \x1b[31mred\x1b[0m\x1b[?1000;2004h tail";
    expect(splitTracking(chunk)).toBe("hello \x1b[31mred\x1b[0m\x1b[?2004h\x1b[?1000h tail");
    expect(splitTracking("plain text")).toBe("plain text");
  });
});
