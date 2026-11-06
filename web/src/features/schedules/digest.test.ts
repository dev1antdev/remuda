import { afterEach, describe, expect, it } from "vitest";
import { setLang } from "@/lib/i18n";
import { digestSchedule, kindOf, SINCE_OPTIONS, sinceWords } from "./digest";

describe("digest schedules", () => {
  afterEach(() => setLang("en"));

  it("reads a record from before the kinds as a prompt", () => {
    expect(kindOf({})).toBe("prompt");
    expect(kindOf({ kind: "prompt" })).toBe("prompt");
    expect(kindOf({ kind: "digest" })).toBe("digest");
  });

  it("names every period the server knows", () => {
    expect(SINCE_OPTIONS.map((o) => o.value)).toEqual(["midnight", "24h", "7d", "last"]);
    expect(sinceWords(undefined)).toBe("since midnight");
    expect(sinceWords("midnight")).toBe("since midnight");
    expect(sinceWords("24h")).toBe("the last 24 h");
    expect(sinceWords("7d")).toBe("the last 7 days");
    expect(sinceWords("last")).toBe("since the previous digest");
  });

  it("says the period in the page's language, as the row around it is", () => {
    setLang("ru");
    expect(sinceWords("midnight")).toBe("с полуночи");
    expect(sinceWords("24h")).toBe("за последние 24 ч");
    expect(sinceWords("7d")).toBe("за последние 7 дней");
    expect(sinceWords("last")).toBe("с предыдущего дайджеста");
  });

  it("builds a digest with no machine, folder or prompt", () => {
    const fresh = digestSchedule({ cron: "0 9 * * 1-5" });
    expect(fresh).toMatchObject({ kind: "digest", machine: "", dir: "", prompt: "", since: "midnight", enabled: true, cron: "0 9 * * 1-5" });
    expect(digestSchedule({ id: "a", since: "7d", enabled: false, label: "Morning" })).toMatchObject({ id: "a", since: "7d", enabled: false, label: "Morning" });
  });
});
