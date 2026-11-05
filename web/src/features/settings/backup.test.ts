import { describe, expect, it } from "vitest";
import { bytesWords, lastWords, nextWords, validTime } from "./backup";

describe("the backups' words", () => {
  it("says a copy's size", () => {
    expect(bytesWords(300)).toBe("1 KB");
    expect(bytesWords(640 * 1024)).toBe("640 KB");
    expect(bytesWords(1.25 * 1024 * 1024)).toBe("1.3 MB");
  });
  it("says where the last run went, or why it did not", () => {
    const now = Date.parse("2026-11-05T12:00:00Z");
    expect(lastWords({ at: "2026-11-05T09:00:00Z", machine: "gog-id", label: "GOG", size: 2048, files: 31, took: 1.2 }, now)).toBe("Taken 3 h ago to GOG · 2 KB · 31 files");
    expect(lastWords({ at: "2026-11-05T11:55:00Z", machine: "gog-id", took: 0, error: "GOG is not connected" }, now)).toBe("Failed 5 min ago, to gog-id: GOG is not connected · no good copy yet");
  });
  it("says how old the newest good copy is while the runs fail", () => {
    const now = Date.parse("2026-11-05T12:00:00Z");
    const good = { at: "2026-11-05T09:00:00Z", machine: "gog-id", label: "GOG", size: 2048, files: 31, took: 1.2 };
    expect(lastWords({ at: "2026-11-05T11:55:00Z", machine: "gog-id", took: 0, error: "GOG is not connected" }, now, good)).toBe("Failed 5 min ago, to gog-id: GOG is not connected · the newest good copy is from 3 h ago");
  });
  it("says when the nightly run comes", () => {
    const now = new Date(2026, 10, 5, 12, 0);
    expect(nextWords(new Date(2026, 10, 5, 23, 30).toISOString(), now)).toMatch(/^next today at /);
    expect(nextWords(new Date(2026, 10, 6, 3, 30).toISOString(), now)).toMatch(/^next tomorrow at /);
    expect(nextWords(new Date(2026, 10, 9, 3, 30).toISOString(), now)).toMatch(/^next on /);
  });
  it("takes a time the server takes", () => {
    expect(validTime("03:30")).toBe(true);
    expect(validTime("3:30")).toBe(false);
    expect(validTime("24:00")).toBe(false);
  });
});
