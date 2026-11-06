import { describe, expect, it } from "vitest";
import { setLang, t } from "@/lib/i18n";
import { clamp, countWords, findRanges, MAX_MATCHES, revealDue, step } from "./find";

describe("findRanges", () => {
  it("finds every occurrence, case-folded, left to right", () => {
    expect(findRanges("Make the Wall; the wall is made", "wall")).toEqual([
      [9, 13],
      [19, 23],
    ]);
  });
  it("never overlaps", () => {
    expect(findRanges("aaaa", "aa")).toEqual([
      [0, 2],
      [2, 4],
    ]);
  });
  it("answers nothing for an empty query or no hit", () => {
    expect(findRanges("anything", "")).toEqual([]);
    expect(findRanges("anything", "zzz")).toEqual([]);
    expect(findRanges("", "a")).toEqual([]);
  });
  it("keeps the offsets of the text as it is when folding changes a length", () => {
    const text = "İstanbul and istanbul";
    const ranges = findRanges(text, "istanbul");
    expect(ranges.length).toBe(1);
    for (const [start, end] of ranges) expect(text.slice(start, end)).toBe("istanbul");
  });
  it("folds the query too", () => {
    expect(findRanges("remuda", "REMUDA")).toEqual([[0, 6]]);
  });
});

describe("step and clamp", () => {
  it("walks forward and back around the ends", () => {
    expect(step(0, 3, 1)).toBe(1);
    expect(step(2, 3, 1)).toBe(0);
    expect(step(0, 3, -1)).toBe(2);
    expect(step(5, 0, 1)).toBe(0);
  });
  it("keeps the current match inside the list", () => {
    expect(clamp(7, 3)).toBe(2);
    expect(clamp(1, 3)).toBe(1);
    expect(clamp(-2, 3)).toBe(0);
    expect(clamp(4, 0)).toBe(0);
  });
});

describe("countWords", () => {
  it("counts from one and says when the bound was hit", () => {
    expect(countWords(0, 0)).toBe("0 of 0");
    expect(countWords(2, 12)).toBe("3 of 12");
    expect(countWords(0, MAX_MATCHES)).toBe(`1 of ${MAX_MATCHES}+`);
  });
  it("counts in the page's language, as does the find toggle's own words", () => {
    setLang("ru");
    try {
      expect(countWords(2, 12)).toBe("3 из 12");
      expect(countWords(0, 0)).toBe("0 из 0");
      expect(t("in this conversation")).toBe("в этом разговоре");
    } finally {
      setLang("en");
    }
  });
});

describe("revealDue", () => {
  const view = { query: "error", at: 0, mounted: true };

  it("brings the match into view when the find opens, the query changes or a step moves it", () => {
    expect(revealDue(null, view)).toBe(true);
    expect(revealDue(view, { ...view, query: "errors" })).toBe(true);
    expect(revealDue(view, { ...view, at: 1 })).toBe(true);
  });

  it("brings it into view again once the earlier turns have mounted, a find opened before they had", () => {
    // reopened on a long conversation with the last query: the first match counted was the first of the last 200 turns
    expect(revealDue({ ...view, mounted: false }, view)).toBe(true);
  });

  it("leaves the view alone on a poll that changed nothing it hangs on, and while the log is still mounting", () => {
    expect(revealDue(view, { ...view })).toBe(false);
    expect(revealDue(view, { ...view, mounted: false })).toBe(false);
  });
});
