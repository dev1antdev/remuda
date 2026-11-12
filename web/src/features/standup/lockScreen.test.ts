import { describe, expect, it } from "vitest";
import { lockTitle, silentWav } from "./lockScreen";

describe("silentWav", () => {
  it("is a mono 8-bit WAV of silence of the length asked", () => {
    const wav = silentWav(2, 8000);
    const view = new DataView(wav.buffer);
    const text = (at: number) => String.fromCharCode(...wav.slice(at, at + 4));
    expect([text(0), text(8), text(12), text(36)]).toEqual(["RIFF", "WAVE", "fmt ", "data"]);
    expect(wav.length).toBe(44 + 16000);
    expect(view.getUint32(4, true)).toBe(wav.length - 8);
    expect([view.getUint16(20, true), view.getUint16(22, true), view.getUint32(24, true), view.getUint16(34, true)]).toEqual([1, 1, 8000, 8]);
    expect(view.getUint32(40, true)).toBe(16000);
    expect(wav.slice(44).every((sample) => sample === 128)).toBe(true);
  });
});

describe("lockTitle", () => {
  it("keeps a short sentence and cuts a long one at a word", () => {
    expect(lockTitle("In api, Claude Code has finished.")).toBe("In api, Claude Code has finished.");
    const long = "It edited roster.go, store.ts and fourteen more files and ran twenty commands on the way to a green build.";
    const cut = lockTitle(long, 60);
    expect(cut.endsWith("…")).toBe(true);
    expect(cut.length).toBeLessThanOrEqual(60);
    expect(long.startsWith(cut.slice(0, -1))).toBe(true);
    expect(cut.slice(0, -1).endsWith(" ")).toBe(false);
  });
});
