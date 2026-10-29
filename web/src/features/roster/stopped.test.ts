import { describe, expect, it } from "vitest";
import { MAX_SEEN, episodeKey, remember, stoppedNames, stoppedWords } from "./stopped";

const stopped = [
  { pane: "w3:p1", agent: "codex", title: "web" },
  { pane: "w1:p1", agent: "claude", title: "api" },
];

describe("stopped chats", () => {
  it("keys an episode by the machine and its panes, in one order", () => {
    expect(episodeKey("local", stopped)).toBe("local:w1:p1,w3:p1");
    expect(episodeKey("local", [...stopped].reverse())).toBe("local:w1:p1,w3:p1");
    expect(episodeKey("gog", stopped.slice(1))).toBe("gog:w1:p1");
  });
  it("says how many and which", () => {
    expect(stoppedWords(1)).toBe("1 chat stopped with the reboot");
    expect(stoppedWords(3)).toBe("3 chats stopped with the reboot");
    expect(stoppedNames(stopped)).toBe("Codex in web, Claude Code in api");
    expect(stoppedNames([{ pane: "w2:p1", agent: "skynet", title: "" }])).toBe("skynet in w2:p1");
  });
  it("keeps the dismissed episodes, the newest last, no more than MAX_SEEN", () => {
    expect(remember([], "a")).toEqual(["a"]);
    expect(remember(["a", "b"], "a")).toEqual(["b", "a"]);
    const many = Array.from({ length: MAX_SEEN }, (_, i) => `k${i}`);
    expect(remember(many, "new")).toHaveLength(MAX_SEEN);
    expect(remember(many, "new").at(-1)).toBe("new");
    expect(remember(many, "new")).not.toContain("k0");
  });
});
