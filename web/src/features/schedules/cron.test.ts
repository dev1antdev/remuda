import { describe, expect, it } from "vitest";
import { cronFor, DEFAULT_CRON, parseCron, type When } from "./cron";

const at = (preset: When["preset"], hh: number, mm: number): When => ({ preset, hh, mm, step: 10, from: 9, until: 17, weekdays: true });
const every = (step: number, from: number, until: number, weekdays: boolean): When => ({ preset: "every", hh: 9, mm: 0, step, from, until, weekdays });

describe("cron presets", () => {
  it("write the cron a preset means", () => {
    expect(cronFor(at("daily", 9, 30))).toBe("30 9 * * *");
    expect(cronFor(at("weekdays", 7, 0))).toBe("0 7 * * 1-5");
    expect(cronFor(at("hourly", 9, 15))).toBe("15 * * * *");
    expect(cronFor(at("every3h", 9, 5))).toBe("5 */3 * * *");
    expect(cronFor(at("custom", 9, 15))).toBeNull();
  });
  it("write every N minutes inside a window of hours, the last run in the hour before it ends", () => {
    expect(cronFor(every(10, 9, 17, true))).toBe("*/10 9-16 * * 1-5");
    expect(cronFor(every(15, 0, 24, false))).toBe("*/15 * * * *");
    expect(cronFor(every(30, 8, 9, false))).toBe("*/30 8 * * *");
    expect(cronFor(every(5, 22, 24, true))).toBe("*/5 22-23 * * 1-5");
  });
  it("read their own cron back, with the time", () => {
    expect(parseCron("30 9 * * *")).toMatchObject({ preset: "daily", hh: 9, mm: 30 });
    expect(parseCron("0 7 * * 1-5")).toMatchObject({ preset: "weekdays", hh: 7, mm: 0 });
    expect(parseCron("15 * * * *")).toMatchObject({ preset: "hourly", hh: 9, mm: 15 });
    expect(parseCron("5 */3 * * *")).toMatchObject({ preset: "every3h", hh: 9, mm: 5 });
    expect(parseCron(DEFAULT_CRON)).toMatchObject({ preset: "weekdays", hh: 9, mm: 0 });
  });
  it("read an every-N-minutes cron back with its window", () => {
    expect(parseCron("*/10 9-16 * * 1-5")).toMatchObject({ preset: "every", step: 10, from: 9, until: 17, weekdays: true });
    expect(parseCron("*/5 * * * *")).toMatchObject({ preset: "every", step: 5, from: 0, until: 24, weekdays: false });
    expect(parseCron("*/30 8 * * *")).toMatchObject({ preset: "every", step: 30, from: 8, until: 9 });
  });
  it("call anything else custom, keeping a readable time", () => {
    expect(parseCron("0 9 1 * *")).toMatchObject({ preset: "custom", hh: 9, mm: 0 });
    expect(parseCron("0 22 * * 0,6")).toMatchObject({ preset: "custom", hh: 22, mm: 0 });
    expect(parseCron("*/7 9-16 * * 1-5").preset).toBe("custom"); // a step the form does not offer
    expect(parseCron("*/10 9-16 * * 0,6").preset).toBe("custom");
    expect(parseCron("*/10 16-9 * * *").preset).toBe("custom");
  });
  it("keep a weekday restriction an hourly cron carries, instead of reading it as every hour of every day", () => {
    expect(parseCron("5 * * * 1-5").preset).toBe("custom");
    expect(parseCron("0 */3 * * 1-5").preset).toBe("custom");
  });
  it("round-trip every preset through its own cron", () => {
    for (const when of [at("daily", 14, 45), at("weekdays", 14, 45), at("hourly", 9, 45), at("every3h", 9, 45), every(20, 7, 19, true), every(5, 0, 24, false)]) {
      const cron = cronFor(when)!;
      expect(parseCron(cron).preset).toBe(when.preset);
      expect(cronFor(parseCron(cron))).toBe(cron);
    }
  });
});
