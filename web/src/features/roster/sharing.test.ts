import { describe, expect, it } from "vitest";
import { DEFAULT_SHARING, extendNames, folderKey, hidesFolder, hidesMachine, letters, redact, sharingOf, switched, toggleMark } from "./sharing";

describe("the pref", () => {
  it("reads damage as off with nothing marked", () => {
    expect(sharingOf(null)).toEqual(DEFAULT_SHARING);
    expect(sharingOf({ on: "yes", folders: ["a", 3], machines: "x" })).toEqual({ on: false, all: false, folders: ["a"], machines: [] });
  });

  it("hides only while on, a machine with all its folders, everything with all", () => {
    const p = { on: true, all: false, folders: [folderKey("gog", "/p/games-db")], machines: ["roofr"] };
    expect(hidesFolder(p, "gog", "/p/games-db")).toBe(true);
    expect(hidesFolder(p, "gog", "/p/sections")).toBe(false);
    expect(hidesFolder(p, "roofr", "/p/roofr")).toBe(true);
    expect(hidesMachine(p, "roofr")).toBe(true);
    expect(hidesMachine(p, "gog")).toBe(false);
    expect(hidesFolder({ ...p, on: false }, "gog", "/p/games-db")).toBe(false);
    expect(hidesMachine({ ...p, all: true }, "local")).toBe(true);
  });

  it("turns on hiding everything when nothing is marked, and keeps the marks otherwise", () => {
    expect(switched(DEFAULT_SHARING, true)).toEqual({ ...DEFAULT_SHARING, on: true, all: true });
    const marked = { ...DEFAULT_SHARING, folders: ["gog|/p"] };
    expect(switched(marked, true)).toEqual({ ...marked, on: true });
    expect(switched({ ...marked, on: true }, false)).toEqual(marked);
  });

  it("toggles a mark", () => {
    expect(toggleMark(["a"], "b")).toEqual(["a", "b"]);
    expect(toggleMark(["a", "b"], "a")).toEqual(["b"]);
  });
});

describe("neutral names", () => {
  it("counts letters as spreadsheet columns", () => {
    expect([0, 1, 25, 26, 27, 51, 52].map(letters)).toEqual(["A", "B", "Z", "AA", "AB", "AZ", "BA"]);
  });

  it("keeps a name once given and gives new keys the next ones", () => {
    const name = (n: number) => `P${n}`;
    const first = extendNames(new Map(), ["x", "y", "x"], name);
    expect([...first]).toEqual([
      ["x", "P0"],
      ["y", "P1"],
    ]);
    expect(extendNames(first, ["y", "x"], name)).toBe(first);
    const second = extendNames(first, ["z", "x"], name);
    expect([...second]).toEqual([
      ["x", "P0"],
      ["y", "P1"],
      ["z", "P2"],
    ]);
  });
});

describe("redact", () => {
  it("swaps the longest names first, everywhere in the text", () => {
    const pairs: [string, string][] = [
      ["user", "Project B"],
      ["user-profiles", "Project A"],
      ["GOG", "Machine 2"],
    ];
    expect(redact("user-profiles on GOG finished; GOG again", pairs)).toBe("Project A on Machine 2 finished; Machine 2 again");
  });

  it("leaves names too short to swap safely", () => {
    expect(redact("an api call in api", [["api", "Project A"], ["ap", "Project B"]])).toBe("an Project A call in Project A");
    expect(redact("map", [["ap", "X"]])).toBe("map");
  });
});
