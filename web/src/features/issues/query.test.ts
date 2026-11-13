import { describe, expect, it } from "vitest";
import { asked, chipOf, filterRows, hasToken, labelsIn, labelToken, MINE_QUERY, OPEN_QUERY, pageCount, pageOf, pagerPages, sortOf, stateOf, toggleLabel, toggleToken, tokensOf, withSort, withState } from "./query";

describe("the search's qualifiers", () => {
  it("are words, a quoted phrase kept whole", () => {
    expect(tokensOf('is:issue label:"good first issue" crash')).toEqual(["is:issue", 'label:"good first issue"', "crash"]);
  });
  it("toggle in and out at the end", () => {
    expect(toggleToken(OPEN_QUERY, "assignee:@me")).toBe(MINE_QUERY);
    expect(toggleToken(MINE_QUERY, "assignee:@me")).toBe(OPEN_QUERY);
    expect(toggleToken("is:issue  no:assignee   crash", "no:assignee")).toBe("is:issue crash");
    expect(hasToken("is:issue Assignee:@me", "assignee:@me")).toBe(true);
  });
  it("hold one state, after is:issue", () => {
    expect(stateOf(OPEN_QUERY)).toBe("open");
    expect(withState(OPEN_QUERY, "closed")).toBe("is:issue is:closed");
    expect(withState("is:issue is:closed crash", "all")).toBe("is:issue crash");
    expect(stateOf("is:issue crash")).toBe("all");
    expect(withState("crash", "open")).toBe("is:open crash");
  });
  it("hold one order", () => {
    expect(sortOf(OPEN_QUERY)).toBe("updated-desc");
    expect(withSort("is:issue sort:created-desc", "comments-desc")).toBe("is:issue sort:comments-desc");
    expect(sortOf("is:issue sort:Comments-desc")).toBe("comments-desc");
  });
  it("quote a label with a blank", () => {
    expect(labelToken("bug")).toBe("label:bug");
    expect(labelToken("good first issue")).toBe('label:"good first issue"');
    expect(toggleLabel(OPEN_QUERY, "good first issue")).toBe('is:issue is:open label:"good first issue"');
    expect(toggleLabel('is:issue is:open label:"good first issue"', "good first issue")).toBe(OPEN_QUERY);
  });
  it("read as a chip only when they are one exactly", () => {
    expect(chipOf(" is:issue  is:open ")).toBe("open");
    expect(chipOf(MINE_QUERY)).toBe("mine");
    expect(chipOf("is:issue is:open crash")).toBeNull();
    expect(asked("   ")).toBe(OPEN_QUERY);
  });
});

describe("the Filters menu", () => {
  it("checks what the query holds and says what each row would make it", () => {
    const rows = filterRows("is:issue is:open assignee:@me sort:created-desc", ["bug", "good first issue"]);
    const row = (id: string) => rows.find((r) => "id" in r && r.id === id) as { checked: boolean; query: string };
    expect(row("open").checked).toBe(true);
    expect(row("open").query).toBe("is:issue assignee:@me sort:created-desc");
    expect(row("closed").query).toBe("is:issue is:closed assignee:@me sort:created-desc");
    expect(row("mine").checked).toBe(true);
    expect(row("mine").query).toBe("is:issue is:open sort:created-desc");
    expect(row("created").checked).toBe(true);
    expect(row("updated").checked).toBe(false);
    expect(row("label:good first issue").query).toBe('is:issue is:open assignee:@me sort:created-desc label:"good first issue"');
    expect(row("reset").query).toBe(OPEN_QUERY);
    expect(rows.some((r) => "sep" in r)).toBe(true);
  });
  it("has no labels heading when the list wears none", () => {
    expect(filterRows(OPEN_QUERY, []).some((r) => "heading" in r && r.heading === "Labels")).toBe(false);
  });
  it("offers the most worn labels first", () => {
    const items = [{ labels: [{ name: "bug" }, { name: "ui" }] }, { labels: [{ name: "ui" }] }, { labels: [{ name: "api" }] }];
    expect(labelsIn(items)).toEqual(["ui", "api", "bug"]);
    expect(labelsIn(items, 1)).toEqual(["ui"]);
  });
});

describe("the pages", () => {
  const items = Array.from({ length: 60 }, (_, i) => i);
  it("hold 25 a page, the last what is left", () => {
    expect(pageCount(0)).toBe(1);
    expect(pageCount(60)).toBe(3);
    expect(pageOf(items, 2)).toHaveLength(10);
    expect(pageOf(items, 9)[0]).toBe(50);
  });
  it("number five around the one on screen", () => {
    expect(pagerPages(0, 3)).toEqual([0, 1, 2]);
    expect(pagerPages(5, 8)).toEqual([3, 4, 5, 6, 7]);
    expect(pagerPages(7, 8)).toEqual([3, 4, 5, 6, 7]);
    expect(pagerPages(3, 10)).toEqual([1, 2, 3, 4, 5]);
  });
});
