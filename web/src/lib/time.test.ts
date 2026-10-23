import { describe, expect, it } from "vitest";
import { ago, elapsed, spent } from "./time";

describe("ago", () => {
  const now = Date.UTC(2026, 9, 13, 12, 0, 0);
  const minutes = (n: number) => now - n * 60_000;
  it("says just now, minutes, hours, days", () => {
    expect(ago(minutes(0.4), now)).toBe("just now");
    expect(ago(minutes(5), now)).toBe("5 min ago");
    expect(ago(minutes(3 * 60), now)).toBe("3 h ago");
    expect(ago(minutes(47 * 60), now)).toBe("47 h ago");
    expect(ago(minutes(48 * 60), now)).toBe("2 days ago");
  });
  it("takes a Date, a string or a number", () => {
    expect(ago(new Date(minutes(10)), now)).toBe("10 min ago");
    expect(ago(new Date(minutes(10)).toISOString(), now)).toBe("10 min ago");
  });
});

describe("elapsed", () => {
  it("says how long something has been going on", () => {
    const now = 1_700_000_000_000;
    expect(elapsed(now - 40_000, now)).toBe("40 s");
    expect(elapsed(now - 6 * 60_000, now)).toBe("6 min");
    expect(elapsed(now - 72 * 60_000, now)).toBe("1 h 12 min");
    expect(elapsed(now - 180 * 60_000, now)).toBe("3 h");
    expect(elapsed(now - 49 * 3_600_000, now)).toBe("2 days");
    expect(elapsed(now + 5_000, now)).toBe("0 s");
  });
});

describe("spent", () => {
  it("says how long a piece of work took", () => {
    expect(spent(12.4)).toBe("12 s");
    expect(spent(252)).toBe("4 min 12 s");
    expect(spent(300)).toBe("5 min");
    expect(spent(4320)).toBe("1 h 12 min");
    expect(spent(7200)).toBe("2 h");
    expect(spent(-3)).toBe("0 s");
  });
});
