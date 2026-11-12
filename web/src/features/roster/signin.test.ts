import { describe, expect, it } from "vitest";
import { concerns, hostConcerns, updateLabel } from "./concerns";
import { BAD_DAYS, LOOK_DAYS, loginWords, signinConcern } from "./signin";
import type { Health } from "./api";

const DAY = 86_400_000;
const now = Date.parse("2026-11-12T12:00:00Z");
const endingIn = (ms: number) => ({ expires: new Date(now + ms).toISOString(), at: new Date(now).toISOString() });
const health = (over: Partial<Health>): Health => ({ at: "", disk_free_gb: 100, disk_used_pct: 10, load1: 0, ...over });

describe("signinConcern", () => {
  it("says nothing while more than ten days are left or no end is known", () => {
    expect(signinConcern(endingIn(LOOK_DAYS * DAY + 60_000), "GOG", now)).toBeNull();
    expect(signinConcern(undefined, "GOG", now)).toBeNull();
    expect(signinConcern({ expires: "soon", at: "" }, "GOG", now)).toBeNull();
  });

  it("is a look from ten days, bad from two, and says where to sign in again", () => {
    expect(signinConcern(endingIn(6 * DAY), "GOG", now)).toEqual({ text: "Claude Code's sign-in on GOG ends in 6 days: run claude auth login there before then", bad: false });
    expect(signinConcern(endingIn(BAD_DAYS * DAY + 3_600_000), "GOG", now)?.bad).toBe(false);
    expect(signinConcern(endingIn(BAD_DAYS * DAY), "GOG", now)?.bad).toBe(true);
    expect(signinConcern(endingIn(20 * 3_600_000), "GOG", now)?.text).toBe("Claude Code's sign-in on GOG ends in 20 h: run claude auth login there before then");
  });

  it("is bad once it has ended", () => {
    expect(signinConcern(endingIn(-3 * 3_600_000), "GOG", now)).toEqual({ text: "Claude Code's sign-in on GOG ended 3 h ago: its agents cannot reach Claude until you run claude auth login there", bad: true });
  });
});

describe("the sign-in among a machine's concerns", () => {
  it("comes first after the stopped chats, with a shell there as its button", () => {
    const h = health({ claude: "2.1.322", claude_login: endingIn(DAY) });
    const list = concerns(h, { at: "", claude: "2.1.330" }, "v0.13.0", false, [], false, { machine: "GOG", now });
    expect(list.map((c) => c.key)).toEqual(["signin", "claude"]);
    expect(list[0]).toMatchObject({ bad: true, update: "signin" });
    expect(updateLabel("signin")).toBe("Open a shell there");
    expect(concerns(h, undefined, "v0.13.0", true, [], true, { machine: "GOG", now })[0]).not.toHaveProperty("update");
  });

  it("is none of a server's business", () => {
    expect(hostConcerns(health({ claude: "2.1.322", claude_login: endingIn(-DAY) }), undefined, "v0.13.0")).toEqual([]);
  });
});

describe("loginWords", () => {
  it("says the day and how long is left, or that it ended", () => {
    expect(loginWords(endingIn(23 * DAY), now)).toMatch(/^ends .*2026 · in 23 days$/);
    expect(loginWords(endingIn(-DAY), now)).toMatch(/^ended .*2026$/);
    expect(loginWords(undefined, now)).toBe("");
  });
});
