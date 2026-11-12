import { afterEach, describe, expect, it } from "vitest";
import { setLang } from "./i18n";
import { ago, clockOf, elapsed, fmtDateSpan, spent, took, whenAhead } from "./time";

afterEach(() => setLang("en"));

describe("fmtDateSpan", () => {
  it("writes two days of one month as one span, the year once", () => {
    const span = fmtDateSpan(new Date(2026, 11, 2, 12), new Date(2026, 11, 5, 12));
    expect(span).toMatch(/2.*5/);
    expect(span.match(/2026/g)).toHaveLength(1);
  });
});

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

describe("took", () => {
  it("keeps a tenth under ten seconds and reads as spent above", () => {
    expect(took(0.04)).toBe("0.0 s");
    expect(took(0.84)).toBe("0.8 s");
    expect(took(1.25)).toBe("1.3 s");
    expect(took(9.96)).toBe("10.0 s");
    expect(took(12.4)).toBe("12 s");
    expect(took(252)).toBe("4 min 12 s");
  });
  it("writes the tenth with the language's decimal mark", () => {
    setLang("ru");
    expect(took(0.84)).toBe("0,8 с");
    expect(took(12.4)).toBe("12 с");
  });
});

describe("whenAhead", () => {
  const now = new Date(2026, 10, 13, 16, 35); // a Friday
  const clock = (d: Date) => d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  it("says the time today, the weekday within the week, the date past it — always with the time", () => {
    const today = new Date(2026, 10, 13, 16, 50);
    const monday = new Date(2026, 10, 16, 9, 0);
    const later = new Date(2026, 10, 30, 9, 0);
    expect(whenAhead(today, now)).toBe(clock(today));
    expect(whenAhead(monday, now)).toBe(`${monday.toLocaleDateString([], { weekday: "short" })} ${clock(monday)}`);
    expect(whenAhead(later, now)).toBe(`${later.toLocaleDateString([], { day: "numeric", month: "short" })} ${clock(later)}`);
  });
});

describe("clockOf", () => {
  it("reads a span as a player's clock does", () => {
    expect([0, 7_400, 754_000, 3_723_000, -5].map(clockOf)).toEqual(["0:00", "0:07", "12:34", "1:02:03", "0:00"]);
  });
});
