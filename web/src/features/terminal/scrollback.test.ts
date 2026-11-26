import { describe, expect, it } from "vitest";
import viewers from "../../../../internal/core/viewers.go?raw";
import { olderRead, tellsOlder, XTERM_SCROLLBACK } from "./scrollback";

describe("XTERM_SCROLLBACK", () => {
  it("is the history a viewer on remuda-core is painted on attach", () => {
    expect(viewers).toMatch(new RegExp(`const attachHistory = ${XTERM_SCROLLBACK}\\b`));
  });
});

describe("olderRead", () => {
  it("reads once for a pane and query, however many steps ask", async () => {
    let reads = 0;
    const read = () => Promise.resolve({ matches: [{ row: 1, col: 0, len: 1 }].slice(0, ++reads), rows: 10, truncated: true });
    const first = olderRead(null, "m|p|err", read);
    const again = olderRead(first, "m|p|err", read);
    expect(again).toBe(first);
    expect(await again.matches).toBe(1);
    expect((await again.found).truncated).toBe(true);
    expect(reads).toBe(1);
  });
  it("reads anew for another query or pane", () => {
    let reads = 0;
    const read = () => { reads++; return Promise.resolve({ matches: [], rows: 10, truncated: false }); };
    const first = olderRead(null, "m|p|err", read);
    expect(olderRead(first, "m|p|warn", read)).not.toBe(first);
    expect(olderRead(first, "m|q|err", read)).not.toBe(first);
    expect(reads).toBe(3);
  });
});

describe("tellsOlder", () => {
  const read = { key: "k", matches: Promise.resolve(2), found: Promise.resolve({ matches: [], rows: 0, truncated: false }), told: false };
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
