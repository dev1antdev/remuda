import { describe, expect, it } from "vitest";
import { ru } from "./ru";

const slots = (s: string) => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1]).sort();

describe("the Russian dictionary", () => {
  const entries = Object.entries(ru);
  it("has words for every key, none empty", () => {
    expect(entries.length).toBeGreaterThan(800);
    for (const [key, value] of entries) {
      for (const form of Array.isArray(value) ? value : [value]) expect(form.trim(), key).not.toBe("");
    }
  });
  it("keeps every placeholder of the key in the value, or in each of its forms", () => {
    for (const [key, value] of entries) {
      const wanted = slots(key);
      for (const form of Array.isArray(value) ? value : [value]) expect(slots(form), key).toEqual(wanted);
    }
  });
  it("gives a form table three forms: one, few, many", () => {
    for (const [key, value] of entries) if (Array.isArray(value)) expect(value, key).toHaveLength(3);
  });
  it("has the shell's and the pieces' words that were still English: the header's tooltips, the Wall, a tile, the viewer bar, a code fence", () => {
    for (const key of [
      "Search every conversation on every machine (⌘K / Ctrl+K has it too)",
      "{model}: context {used} of {window} ({pct}%) · {responses} · {cost} API-equivalent so far.",
      "Click for the usage report.",
      "Show {n} hidden",
      "Leave office mode (Esc)",
      "Back to the grid (Esc)",
      "Open in a new tab",
      "A subagent of",
      "A transcript from",
      "copy",
      "copied",
      "on the Wall ({tiles})",
      "Rename tab {tab}…",
      "{workspace} on {machine}",
      "{n} agents",
    ])
      expect(ru[key], key).toBeTruthy();
  });
  it("translates a key that counts into a form table, so the count picks its form", () => {
    for (const [key, value] of entries) if (/^\{n\} (files|tabs|tasks|agents|days)$/.test(key)) expect(Array.isArray(value), key).toBe(true);
  });
});
