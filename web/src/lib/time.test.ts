import { describe, expect, it } from "vitest";
import { ago } from "./time";

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
