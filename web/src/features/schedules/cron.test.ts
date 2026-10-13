import { describe, expect, it } from "vitest";
import { cronFor, DEFAULT_CRON, parseCron } from "./cron";

describe("cron presets", () => {
  it("write the cron a preset means", () => {
    expect(cronFor("daily", 9, 30)).toBe("30 9 * * *");
    expect(cronFor("weekdays", 7, 0)).toBe("0 7 * * 1-5");
    expect(cronFor("hourly", 9, 15)).toBe("15 * * * *");
    expect(cronFor("custom", 9, 15)).toBeNull();
  });
  it("read their own cron back, with the time", () => {
    expect(parseCron("30 9 * * *")).toEqual({ preset: "daily", hh: 9, mm: 30 });
    expect(parseCron("0 7 * * 1-5")).toEqual({ preset: "weekdays", hh: 7, mm: 0 });
    expect(parseCron("15 * * * *")).toEqual({ preset: "hourly", hh: 9, mm: 15 });
    expect(parseCron(DEFAULT_CRON)).toEqual({ preset: "weekdays", hh: 9, mm: 0 });
  });
  it("call anything else custom, keeping a readable time", () => {
    expect(parseCron("0 9 1 * *")).toEqual({ preset: "custom", hh: 9, mm: 0 });
    expect(parseCron("*/5 * * * *")).toEqual({ preset: "custom", hh: 9, mm: 0 });
    expect(parseCron("0 22 * * 0,6")).toEqual({ preset: "custom", hh: 22, mm: 0 });
  });
});
