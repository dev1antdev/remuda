import { describe, expect, it } from "vitest";
import { leftWords, muteChoices, mutedUntil, type MuteChoice } from "./mute";

describe("mute", () => {
  it("offers an hour and the next morning at least an hour away", () => {
    const evening = new Date(2026, 9, 29, 18, 30);
    const [hour, morning] = muteChoices(evening) as [MuteChoice, MuteChoice];
    expect(hour.until.getTime() - evening.getTime()).toBe(3_600_000);
    expect(morning.label).toBe("Mute until tomorrow 09:00");
    expect(morning.until.getDate()).toBe(30);
    expect(morning.until.getHours()).toBe(9);

    const early = new Date(2026, 9, 29, 6, 0);
    const [, sameDay] = muteChoices(early) as [MuteChoice, MuteChoice];
    expect(sameDay.label).toBe("Mute until 09:00");
    expect(sameDay.until.getDate()).toBe(29);

    const almostNine = new Date(2026, 9, 29, 8, 30);
    const [, nextDay] = muteChoices(almostNine) as [MuteChoice, MuteChoice];
    expect(nextDay.until.getDate()).toBe(30);
  });

  it("reads the roster's mute only while it still holds", () => {
    const now = new Date(2026, 9, 29, 18, 0);
    expect(mutedUntil({}, now)).toBeNull();
    expect(mutedUntil({ muted_until: new Date(2026, 9, 29, 19, 0).toISOString() }, now)?.getHours()).toBe(19);
    expect(mutedUntil({ muted_until: new Date(2026, 9, 29, 17, 0).toISOString() }, now)).toBeNull();
  });

  it("says how long is left", () => {
    const now = new Date(2026, 9, 29, 18, 0);
    expect(leftWords(new Date(2026, 9, 29, 18, 42), now)).toBe("42 min left");
    expect(leftWords(new Date(2026, 9, 29, 21, 10), now)).toBe("3 h 10 min left");
    expect(leftWords(new Date(2026, 9, 29, 20, 0), now)).toBe("2 h left");
    expect(leftWords(new Date(2026, 9, 29, 18, 0, 20), now)).toBe("1 min left");
    expect(leftWords(new Date(2026, 10, 2, 9, 0), now)).toMatch(/^until /);
  });
});
