import { describe, expect, it } from "vitest";
import { keyGroups } from "./keys";

describe("keyGroups", () => {
  it("names every key and what it does, in groups with distinct titles", () => {
    const groups = keyGroups(true);
    expect(new Set(groups.map((g) => g.title)).size).toBe(groups.length);
    for (const g of groups) {
      expect(g.rows.length).toBeGreaterThan(0);
      for (const row of g.rows) {
        expect(row.keys.length).toBeGreaterThan(0);
        expect(row.what.length).toBeGreaterThan(2);
      }
    }
  });

  it("lists the keys of the composer's completion, the find bar's steps and the review's comment box", () => {
    const rows = keyGroups(true).flatMap((g) => g.rows.map((r) => `${g.title}: ${r.keys.join(" ")}`));
    expect(rows).toContain("Chat: ↑ ↓ ↵ Tab Esc");
    expect(rows).toContain("Chat: ↵ ⇧ ↵ Esc");
    expect(rows).toContain("Changes: ⌘ ↵");
  });

  it("says ⌘ on a Mac and Ctrl elsewhere", () => {
    const flat = (mac: boolean) => keyGroups(mac).flatMap((g) => g.rows.flatMap((r) => r.keys));
    expect(flat(true)).toContain("⌘ K");
    expect(flat(false)).toContain("Ctrl K");
    expect(flat(false).some((k) => k.includes("⌘"))).toBe(false);
  });

  it("leaves out for a viewer the keys of what a viewer has not got, and keeps finding and moving about", () => {
    const rows = (viewOnly: boolean) => keyGroups(true, viewOnly).flatMap((g) => g.rows.map((r) => `${g.title}: ${r.keys.join(" ")}`));
    const viewer = rows(true);
    expect(viewer).toContain("Everywhere: ⌘ K");
    expect(viewer).toContain("Everywhere: Alt N");
    expect(viewer).toContain("Chat: ⌘ F");
    expect(viewer).toContain("Wall: Esc");
    expect(viewer).not.toContain("Everywhere: Alt Z");
    expect(viewer).not.toContain("Everywhere: ⌃ `");
    expect(viewer).not.toContain("Chat: ↵");
    expect(viewer).not.toContain("Chat: Q");
    expect(viewer.some((r) => r.startsWith("Changes:") || r.startsWith("Tasks:") || r.startsWith("Terminal:"))).toBe(false);
    expect(rows(false).length).toBeGreaterThan(viewer.length);
    expect(rows(false)).toContain("Everywhere: Alt Z");
  });
});
