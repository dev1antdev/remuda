import { describe, expect, it } from "vitest";
import type { Dictionary, Forms } from "../i18n";
import { ru } from "./ru";

// every area file of the Russian dictionary, by its name; ru.ts joins them with spreads, the later one winning
const areas = import.meta.glob<Record<string, Dictionary>>("./ru/*.ts", { eager: true });

const same = (a: Forms, b: Forms) => JSON.stringify(a) === JSON.stringify(b);

describe("the Russian dictionary's area files", () => {
  it("are every one joined into ru", () => {
    for (const [file, mod] of Object.entries(areas)) {
      for (const dict of Object.values(mod)) {
        const missing = Object.keys(dict).filter((key) => !(key in ru));
        expect(missing, `${file} is not spread into ru.ts`).toEqual([]);
      }
    }
  });
  it("never give one English string two different translations", () => {
    const seen = new Map<string, { file: string; words: Forms }>();
    const clashes: string[] = [];
    for (const [file, mod] of Object.entries(areas)) {
      for (const dict of Object.values(mod)) {
        for (const [key, words] of Object.entries(dict)) {
          const before = seen.get(key);
          if (before && !same(before.words, words)) clashes.push(`${JSON.stringify(key)}: ${before.file} ${JSON.stringify(before.words)} vs ${file} ${JSON.stringify(words)}`);
          else if (!before) seen.set(key, { file, words });
        }
      }
    }
    expect(clashes).toEqual([]);
  });
});
