import { describe, expect, it } from "vitest";
import type { Target } from "./api";
import { TYPED, firstFolder, firstTarget, folderChoices, folderOf, targetHint, whyNot } from "./teleport";

const clone = (dir: string, same = true) => ({ dir, branch: "main", remote: "git@x:a/b", same });
const mac: Target = { machine: "local", label: "This Mac", checkouts: [clone("/Users/me/b2")] };
const gog: Target = { machine: "gog", label: "GOG", checkouts: [clone("/Users/g/other", false), clone("/Users/g/b")] };
const roofr: Target = { machine: "roofr", label: "Roofr", checkouts: [] };

describe("firstTarget", () => {
  it("prefers another machine with a clone, then this one's other clone, then any other machine", () => {
    expect(firstTarget([mac, gog, roofr], "local")).toBe("gog");
    expect(firstTarget([mac, roofr], "local")).toBe("local");
    expect(firstTarget([{ ...mac, checkouts: [] }, roofr], "local")).toBe("roofr");
    expect(firstTarget([], "local")).toBe("");
  });
});

describe("the folder", () => {
  it("lists the target's clones of the repository, then a path to type", () => {
    expect(folderChoices(gog).map((c) => c.value)).toEqual(["/Users/g/b", TYPED]);
    expect(folderChoices(undefined).map((c) => c.value)).toEqual([TYPED]);
    expect(firstFolder(gog)).toBe("/Users/g/b");
    expect(firstFolder(roofr)).toBe(TYPED);
    expect(folderOf(TYPED, "  /w/app ")).toBe("/w/app");
    expect(folderOf("/Users/g/b", "/ignored")).toBe("/Users/g/b");
  });

  it("says what stops a move", () => {
    expect(whyNot(undefined, "/a", "claude")).toMatch(/machine/);
    expect(whyNot(gog, "", "claude")).toMatch(/GOG/);
    expect(whyNot(gog, "w/app", "claude")).toMatch(/full path/);
    expect(whyNot(gog, "/w/app", "")).toMatch(/agent/);
    expect(whyNot(gog, "/w/app", "claude")).toBe("");
  });

  it("tells what a machine offers", () => {
    expect(targetHint(gog)).toBe("1 clone of this repository");
    expect(targetHint(roofr)).toBe("no clone of this repository open");
    expect(targetHint({ ...roofr, error: "ssh: no route" })).toBe("ssh: no route");
  });
});
