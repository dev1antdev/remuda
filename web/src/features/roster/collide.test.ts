import { describe, expect, it } from "vitest";
import type { Collision } from "./api";
import { collisionWords, gravest, markWords, orderWords, othersCount, whereWords } from "./collide";

const collision = (over: Partial<Collision> = {}): Collision => ({
  with: { machine: "local", workspace: "w2", name: "web", dir: "/w/web" },
  files: [{ path: "src/auth.ts", lines: true, from: 40, to: 58 }],
  lines: true,
  first: false,
  why: "pr",
  main: "origin/main",
  ...over,
});

describe("the collision in words", () => {
  it("says where both change a file", () => {
    expect(whereWords({ path: "a.ts", lines: true, from: 40, to: 58 })).toBe("a.ts, lines 40–58");
    expect(whereWords({ path: "a.ts", lines: true, from: 7, to: 7 })).toBe("a.ts, line 7");
    expect(whereWords({ path: "logo.png", lines: true })).toBe("logo.png (the whole file)");
    expect(whereWords({ path: "a.ts", lines: false })).toBe("a.ts (other lines)");
  });

  it("is a quiet mark naming the other and the first file", () => {
    expect(markWords(collision(), "web")).toBe("same lines as web in src/auth.ts");
    expect(markWords(collision({ lines: false, files: [{ path: "a.ts", lines: false }, { path: "b.ts", lines: false }], more: 2 }), "web")).toBe("collides with web on a.ts and 3 more");
    expect(othersCount(collision({ files: [], more: 0 }))).toBe(0);
  });

  it("says which lands first, from either side", () => {
    expect(orderWords(collision(), "web")).toBe("Land web first: it has a pull request open; this one rebases on it.");
    expect(orderWords(collision({ first: true, why: "smaller" }), "web")).toBe("This one lands first: its change is the smaller one; web rebases on it.");
    expect(orderWords(collision({ first: true, why: "name" }), "web")).toBe("This one lands first; web rebases on it.");
  });

  it("names the files, counts the rest, and ends with the order", () => {
    const files = Array.from({ length: 8 }, (_, i) => ({ path: `f${i}.ts`, lines: false }));
    const lines = collisionWords(collision({ lines: false, files, more: 4, why: "rest" }), "web");
    expect(lines[0]).toBe("Changes the same files as web, on other lines, against origin/main:");
    expect(lines.slice(1, 7)).toEqual(files.slice(0, 6).map((file) => `· ${file.path} (other lines)`));
    expect(lines[7]).toBe("· and 6 more files");
    expect(lines[8]).toBe("Land web first: its agents have finished; this one rebases on it.");
  });

  it("speaks for the same lines before the same files", () => {
    const files = collision({ lines: false });
    const same = collision({ with: { machine: "gog", workspace: "w9", name: "api", dir: "/srv/api" } });
    expect(gravest([files, same])).toBe(same);
    expect(gravest([files])).toBe(files);
    expect(gravest(undefined)).toBeUndefined();
  });
});
