import { describe, expect, it } from "vitest";
import { MAX_SEEN, concernWords, countWords, episodeKey, fromHerdr, herdrWords, remember, stoppedNames, stoppedWords, toastWords } from "./stopped";

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
  it("tells herdr's agents apart from what a reboot stopped", () => {
    const herdr = [
      { pane: "w4:p1", agent: "claude", title: "notes", from: "herdr" as const },
      { pane: "w4:p2", agent: "codex", title: "notes", from: "herdr" as const },
    ];
    expect(fromHerdr(herdr[0]!)).toBe(true);
    expect(fromHerdr(stopped[0]!)).toBe(false);
    expect(herdrWords(1, "GOG")).toBe("1 agent from herdr on GOG can carry on");
    expect(herdrWords(2, "GOG")).toBe("2 agents from herdr on GOG can carry on");
    expect(toastWords(herdr, "GOG")).toBe("2 agents from herdr on GOG can carry on");
    expect(toastWords(stopped, "GOG")).toBe("GOG: 2 chats stopped with the reboot");
    expect(toastWords([...stopped, herdr[0]!], "GOG")).toBe("GOG: 2 chats stopped with the reboot · 1 agent from herdr on GOG can carry on");
    expect(countWords([...stopped, herdr[0]!], "GOG")).toBe("2 chats stopped with the reboot · 1 agent from herdr on GOG can carry on");
    expect(concernWords(herdr, "GOG")).toBe("2 agents from herdr on GOG can carry on: Claude Code in notes, Codex in notes");
    expect(concernWords([stopped[1]!, herdr[1]!], "GOG")).toBe("1 chat stopped with the reboot: Claude Code in api · 1 agent from herdr on GOG can carry on: Codex in notes");
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
