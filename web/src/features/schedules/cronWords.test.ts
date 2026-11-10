import { afterEach, describe, expect, it } from "vitest";
import { setLang } from "@/lib/i18n";
import { cronWords, hostClockWords, offsetOf, parseFields } from "./cronWords";

afterEach(() => setLang("en"));

describe("a cron in words", () => {
  it("says a workday window of every few minutes", () => {
    expect(cronWords("*/10 9-16 * * 1-5")).toBe("Mon–Fri, every 10 min, 9:00–16:50");
    expect(cronWords("*/15 * * * *")).toBe("Every day, every 15 min");
    expect(cronWords("*/30 8 * * *")).toBe("Every day, every 30 min, 8:00–8:30");
    expect(cronWords("5/10 9-16 * * 1-5")).toBe("Mon–Fri, every 10 min from :05, 9:05–16:55");
    expect(cronWords("* 9 * * *")).toBe("Every day, every minute, 9:00–9:59");
  });
  it("says the presets' own times", () => {
    expect(cronWords("0 9 * * 1-5")).toBe("Mon–Fri, at 9:00");
    expect(cronWords("30 9 * * *")).toBe("Every day, at 9:30");
    expect(cronWords("15 * * * *")).toBe("Every day, every hour at :15");
    expect(cronWords("5 */3 * * *")).toBe("Every day, every 3 h at :05");
    expect(cronWords("0 9-17 * * 1-5")).toBe("Mon–Fri, every hour at :00, 9:00–17:00");
  });
  it("says what the form has no preset for", () => {
    expect(cronWords("0 22 * * 0,6")).toBe("Weekends, at 22:00");
    expect(cronWords("0 9,13,17 * * *")).toBe("Every day, at 9:00, 13:00, 17:00");
    expect(cronWords("0,20,45 9-11,14 * * *")).toBe("Every day, at minutes 00, 20, 45, in the hours 9–11, 14");
    expect(cronWords("30 18 1,15 * *")).toBe("On day 1, 15 of the month, at 18:30");
    expect(cronWords("0 9 1 * 1")).toBe("Mon or on day 1 of the month, at 9:00");
    expect(cronWords("0 9 * 1,7 *")).toMatch(/^Every day, in Jan, Jul, at 9:00$/);
    expect(cronWords("0 9 * * 7")).toBe("Sun, at 9:00");
    expect(cronWords("0 9 * * 2-4")).toBe("Tue–Thu, at 9:00");
  });
  it("reads nothing the server would refuse", () => {
    for (const bad of ["", "0 9 * *", "60 9 * * *", "0 25 * * *", "0 9 * * 8", "a b c d e", "0 9 * * */0", "5-1 * * * *", "1- * * * *"]) {
      expect(cronWords(bad)).toBeNull();
    }
  });
  it("keeps the weekday restriction a starred day field leaves alone", () => {
    expect(parseFields("0 9 */2 * 1-5")).toMatchObject({ eitherDay: false });
    expect(parseFields("0 9 1 * 1")).toMatchObject({ eitherDay: true, doms: [1], weekdays: [1] });
  });
  it("speak Russian", () => {
    setLang("ru");
    expect(cronWords("*/10 9-16 * * 1-5")).toBe("Пн–Пт, каждые 10 мин, 9:00–16:50");
    expect(cronWords("0 9 * * *")).toBe("Каждый день, в 9:00");
  });
});

describe("the host's clock", () => {
  it("reads the offset an RFC 3339 time carries", () => {
    expect(offsetOf("2026-11-10T14:10:00+01:00")).toBe(60);
    expect(offsetOf("2026-11-10T14:10:00-05:30")).toBe(-330);
    expect(offsetOf("2026-11-10T13:10:00Z")).toBe(0);
    expect(offsetOf("2026-11-10T14:10:00")).toBeNull();
  });
  it("is named only when it differs from this device's", () => {
    expect(hostClockWords("2026-11-10T14:10:00+01:00", 60)).toBe("");
    expect(hostClockWords("2026-11-10T14:10:00+01:00", -300)).toBe("on the host's clock, UTC+1");
    expect(hostClockWords("2026-11-10T14:10:00+05:30", 0)).toBe("on the host's clock, UTC+5:30");
    expect(hostClockWords("2026-11-10T14:10:00-03:00", 0)).toBe("on the host's clock, UTC−3");
  });
});
