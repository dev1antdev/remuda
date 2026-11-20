import { describe, expect, it } from "vitest";
import viewers from "../../../../internal/core/viewers.go?raw";
import { blankTail, clampAt, fromBottom, nearest, offsetFor, olderRead, onScreen, tellsOlder, viewportRow, XTERM_SCROLLBACK } from "./scrollback";

const scroll = (max: number, rows: number, offset = 0) => ({ offset_from_bottom: offset, max_offset_from_bottom: max, viewport_rows: rows });

describe("blankTail", () => {
  it("is the screen's blank rows under the text when the read was whole", () => {
    expect(blankTail(2, false, scroll(0, 40))).toBe(38); // a fresh prompt at the top of an empty screen
    expect(blankTail(123, false, scroll(83, 40))).toBe(0); // a full screen with 83 rows above it
  });
  it("is none for a truncated read or without the scroll state", () => {
    expect(blankTail(60, true, scroll(83, 40))).toBe(0);
    expect(blankTail(2, false, null)).toBe(0);
  });
});

describe("the rows", () => {
  it("count a match from the bottom of the screen and back to a screen row", () => {
    const fb = fromBottom(100, 123, 0); // row 100 of 123, no blank tail: 22 rows above the bottom
    expect(fb).toBe(22);
    expect(viewportRow(fb, 0, 40)).toBe(17);
    expect(onScreen(viewportRow(fb, 0, 40), 40)).toBe(true);
    expect(onScreen(viewportRow(fromBottom(10, 123, 0), 0, 40), 40)).toBe(false); // 112 rows up: off the top
    expect(viewportRow(22, 30, 40)).toBe(47); // scrolled 30 up, the match went under the bottom
  });
  it("keep the blank tail in the count", () => {
    expect(viewportRow(fromBottom(1, 2, 38), 0, 40)).toBe(1); // the second of two rows on an otherwise empty screen
  });
});

describe("offsetFor", () => {
  it("puts the match a third of the screen above the bottom", () => {
    expect(offsetFor(112, 40)).toBe(99);
    expect(viewportRow(112, offsetFor(112, 40), 40)).toBe(26);
  });
  it("never scrolls past the bottom", () => {
    expect(offsetFor(5, 40)).toBe(0);
  });
});

describe("nearest", () => {
  it("picks the row closest to the one expected, the later on a tie", () => {
    expect(nearest([3, 10, 26, 27], 26)).toBe(2);
    expect(nearest([25, 27], 26)).toBe(1);
    expect(nearest([], 26)).toBe(-1);
  });
});

describe("clampAt", () => {
  it("keeps the index inside the list", () => {
    expect(clampAt(5, 3)).toBe(2);
    expect(clampAt(-1, 3)).toBe(0);
    expect(clampAt(1, 0)).toBe(0);
  });
});

describe("XTERM_SCROLLBACK", () => {
  it("is the history a viewer on remuda-core is painted on attach", () => {
    expect(viewers).toMatch(new RegExp(`const attachHistory = ${XTERM_SCROLLBACK}\\b`));
  });
});

describe("olderRead", () => {
  it("reads once for a pane and query, however many steps ask", async () => {
    let reads = 0;
    const read = () => Promise.resolve(++reads);
    const first = olderRead(null, "m|p|err", read);
    const again = olderRead(first, "m|p|err", read);
    expect(again).toBe(first);
    expect(await again.matches).toBe(1);
    expect(reads).toBe(1);
  });
  it("reads anew for another query or pane", () => {
    let reads = 0;
    const read = () => Promise.resolve(++reads);
    const first = olderRead(null, "m|p|err", read);
    expect(olderRead(first, "m|p|warn", read)).not.toBe(first);
    expect(olderRead(first, "m|q|err", read)).not.toBe(first);
    expect(reads).toBe(3);
  });
});

describe("tellsOlder", () => {
  const read = { key: "k", matches: Promise.resolve(2), told: false };
  it("tells the current run once the older rows hold what the terminal's own rows do not", () => {
    expect(tellsOlder(true, read, 2, 0)).toBe(true);
  });
  it("keeps quiet for a run a step replaced, a read told already, no older match, or matches on the screen meanwhile", () => {
    expect(tellsOlder(false, read, 2, 0)).toBe(false);
    expect(tellsOlder(true, { ...read, told: true }, 2, 0)).toBe(false);
    expect(tellsOlder(true, read, 0, 0)).toBe(false);
    expect(tellsOlder(true, read, 2, 1)).toBe(false);
  });
});
