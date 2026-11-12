import { afterEach, describe, expect, it } from "vitest";
import { setLang } from "@/lib/i18n";
import { keptWords } from "@/features/settings/timelapse";
import type { RecordingMeta } from "./api";
import { durationOf, endedWords, gifName, recordingLabel, shownRecordings, speedWords } from "./words";

const meta = (over: Partial<RecordingMeta> = {}): RecordingMeta => ({
  id: "20261112T140000Z-abcdef",
  machine: "local",
  pane: "w1:p1",
  workspace: "my app",
  dir: "/repo/app",
  folder: "/repo",
  agent: "claude",
  title: "",
  cols: 120,
  rows: 40,
  start: new Date(2026, 10, 12, 14, 2).toISOString(),
  end: new Date(2026, 10, 12, 14, 40).toISOString(),
  count: 300,
  ended: "done",
  size: 1000,
  ...over,
});

afterEach(() => setLang("en"));

describe("a recording's words", () => {
  it("says how long the turn ran and names an earlier pane of the folder", () => {
    expect(durationOf(meta())).toBe(38 * 60_000);
    expect(recordingLabel(meta(), "w1:p1")).toMatch(/ · 38 min · Claude Code$/);
    expect(recordingLabel(meta({ pane: "w3:p2", agent: "codex" }), "w1:p1")).toMatch(/ · Codex · pane w3:p2$/);
  });
  it("says how the turn ended, in the page's language", () => {
    expect(["done", "blocked", "gone", "cut", "odd"].map(endedWords)).toEqual(["finished", "asked you something", "its pane closed", "cut short: remuda went down while it worked", "odd"]);
    setLang("ru");
    expect(endedWords("idle")).toBe("остановился");
    expect(speedWords("frame")).toBe("По кадрам");
    expect(speedWords(60)).toBe("60×");
  });
  it("names the GIF after the workspace, the start and the speed", () => {
    expect(gifName(meta(), 30)).toBe("my-app-2026-11-12-1402-30x.gif");
    expect(gifName(meta({ workspace: "../ «x» " }), "frame")).toBe("x-2026-11-12-1402-frames.gif");
    expect(gifName(meta({ workspace: "" }), 10)).toBe("timelapse-2026-11-12-1402-10x.gif");
  });
});

describe("shownRecordings", () => {
  it("leaves out what the screen-sharing mode hides, by machine and by folder", () => {
    const list = [meta(), meta({ id: "b", folder: "/secret" }), meta({ id: "c", machine: "gog" }), meta({ id: "d", folder: "", dir: "/secret" })];
    const shown = shownRecordings(list, { machine: (id) => id === "gog", folder: (_m, folder) => folder === "/secret" });
    expect(shown.map((r) => r.id)).toEqual(["20261112T140000Z-abcdef"]);
    expect(shownRecordings(list, { machine: () => false, folder: () => false })).toHaveLength(4);
  });
});

describe("keptWords", () => {
  it("says how many recordings take how much of the most there is room for", () => {
    expect(keptWords({ on: true, scratch: false, count: 0, bytes: 0, max: 200 * 1024 * 1024 })).toBe("Nothing kept yet; at most 200.0 MB, the oldest going first");
    expect(keptWords({ on: true, scratch: false, count: 12, bytes: 4.2 * 1024 * 1024, max: 200 * 1024 * 1024 })).toBe("12 recordings, 4.2 MB of at most 200.0 MB; the oldest go first");
    setLang("ru");
    expect(keptWords({ on: true, scratch: false, count: 3, bytes: 2048, max: 200 * 1024 * 1024 })).toBe("3 записи, 2 КБ из не более чем 200,0 МБ; старые удаляются первыми");
  });
});
