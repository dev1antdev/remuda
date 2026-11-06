import { describe, expect, it } from "vitest";
import { baseChoices, checkDraft, DEFAULT_KINDS, draftOf, emptyDraft, hookOf, hookWords, outcomeWords, toggleKind, type Webhook } from "./webhooks";

const quiet = { from: "22:00", to: "07:00" };

const telegram: Webhook = { id: "a1", kind: "telegram", name: "My phone", chat: "42", kinds: ["question", "watchdog"], enabled: true, has_token: true };
const slack: Webhook = { id: "b2", kind: "slack", name: "Team", url: "https://hooks.slack.com/…", kinds: [], enabled: false, quiet: { from: "22:00", to: "07:00", tz: "UTC" } };

describe("hookWords", () => {
  it("names the kind, the chat or address, what it hears and its quiet hours", () => {
    expect(hookWords(telegram)).toBe("Telegram bot · chat 42 · a question, a watchdog note");
    expect(hookWords(slack)).toBe("Slack incoming webhook · https://hooks.slack.com/… · hears nothing · quiet 22:00–07:00 · off");
  });
});

describe("outcomeWords", () => {
  const now = Date.parse("2026-10-29T12:00:00Z");
  it("reads the newer of the last success and the last failure", () => {
    expect(outcomeWords(telegram, now)).toEqual({ text: "not tried yet", tone: "none" });
    expect(outcomeWords({ ...telegram, last_ok: "2026-10-29T11:55:00Z" }, now).tone).toBe("ok");
    const failed = { ...telegram, last_ok: "2026-10-29T11:55:00Z", last_error: "answered 404", last_error_at: "2026-10-29T11:58:00Z" };
    expect(outcomeWords(failed, now)).toEqual({ text: "failed 2 min ago: answered 404", tone: "bad" });
    const recovered = { ...failed, last_ok: "2026-10-29T11:59:00Z" };
    expect(outcomeWords(recovered, now).tone).toBe("ok");
  });
});

describe("baseChoices", () => {
  it("offers the tailnet's address and this page's when they differ from the one set", () => {
    expect(baseChoices("", "https://remuda.tail.ts.net/", "http://127.0.0.1:7317")).toEqual(["https://remuda.tail.ts.net", "http://127.0.0.1:7317"]);
    expect(baseChoices("http://127.0.0.1:7317", undefined, "http://127.0.0.1:7317")).toEqual([]);
    expect(baseChoices("https://remuda.tail.ts.net", "https://remuda.tail.ts.net", "https://remuda.tail.ts.net")).toEqual([]);
  });
});

describe("the draft", () => {
  it("starts with every kind but the finishes", () => {
    const d = emptyDraft(quiet);
    expect(d.kinds).toEqual(DEFAULT_KINDS);
    expect(d.kinds).not.toContain("finish");
    expect(checkDraft(d)).toBe("The bot's token is needed.");
  });
  it("opens a stored webhook with its secrets left alone and sends them back that way", () => {
    const d = draftOf(telegram, quiet);
    expect(d.token).toBe("");
    expect(d.hasToken).toBe(true);
    expect(checkDraft(d)).toBe("");
    expect(hookOf(d, "Europe/Kyiv")).toEqual({ id: "a1", kind: "telegram", name: "My phone", chat: "42", kinds: ["question", "watchdog"], enabled: true });
    const s = draftOf(slack, quiet);
    expect(s.url).toBe("https://hooks.slack.com/…");
    expect(s.quietOn).toBe(true);
    expect(hookOf(s, "Europe/Kyiv")).toEqual({ id: "b2", kind: "slack", name: "Team", url: "https://hooks.slack.com/…", kinds: [], enabled: false, quiet: { from: "22:00", to: "07:00", tz: "Europe/Kyiv" } });
  });
  it("sends a typed token and refuses what the server would", () => {
    const d = { ...draftOf(telegram, quiet), token: " 123:ABC " };
    expect(hookOf(d, "UTC").token).toBe("123:ABC");
    expect(checkDraft({ ...emptyDraft(quiet), kind: "ntfy", url: "ntfy.sh/topic" })).toBe("The address must start with http:// or https://");
    expect(checkDraft({ ...emptyDraft(quiet), kind: "ntfy", url: "https://ntfy.sh/topic" })).toBe("");
    expect(checkDraft({ ...emptyDraft(quiet), token: "t", chat: "" })).toBe("The chat id is needed.");
  });
  it("toggles a kind keeping the panel's order", () => {
    expect(toggleKind(["watchdog"], "question", true)).toEqual(["question", "watchdog"]);
    expect(toggleKind(["question", "watchdog"], "question", false)).toEqual(["watchdog"]);
  });
});
