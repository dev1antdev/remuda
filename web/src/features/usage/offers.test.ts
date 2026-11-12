import { describe, expect, it } from "vitest";
import { agentLabel, holdWords, MAX_SEEN, modelWords, offersToShow, offerWords, rememberOffer, runLabel, whatWaits, type PlanOffer } from "./offers";

const offer = (over: Partial<PlanOffer> = {}): PlanOffer => ({
  id: "a1",
  kind: "task",
  project: 1,
  task: 12,
  ref: "#12",
  title: "Fix the login page",
  machine: "gog",
  from: "claude",
  to: "codex",
  why: "Claude's 5 h window is at 96 % until Nov 12 16:50",
  resets_at: "2026-11-12T16:50:00Z",
  at: "2026-11-12T14:00:00Z",
  ...over,
});

describe("an offer's words", () => {
  it("says what waits on whose limit and whom it would run on", () => {
    expect(offerWords(offer())).toBe("Task #12 “Fix the login page” waits for Claude Code's plan limit.");
    expect(runLabel(offer())).toBe("Run on Codex");
    expect(whatWaits(offer({ kind: "schedule", schedule: "s1", title: "Look around", from: "codex", to: "claude" }))).toBe("Schedule “Look around”");
    expect(runLabel(offer({ to: "claude" }))).toBe("Run on Claude Code");
    expect(agentLabel("gemini")).toBe("gemini");
  });

  it("names the model a recipe gives, else the default", () => {
    expect(modelWords(offer({ model: "gpt-6.1" }))).toBe("on gpt-6.1, as your recipe says");
    expect(modelWords(offer())).toBe("on its default model");
  });
});

describe("why the work waits", () => {
  const now = Date.parse("2026-11-12T14:00:00Z");
  const time = (iso: string) => new Date(iso).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit" });
  it("says the held window in the page's words, else the server's", () => {
    expect(holdWords(offer({ window: "5h", used: 96.4, resets_at: "2026-11-12T16:50:00Z" }), now)).toBe(`Claude 5 h is at 96 %, resets ${time("2026-11-12T16:50:00Z")}`);
    expect(holdWords(offer({ window: "week", used: 100, from: "codex", resets_at: "2026-11-12T16:50:00Z" }), now)).toBe(`Codex week is spent, resets ${time("2026-11-12T16:50:00Z")}`);
    expect(holdWords(offer({ window: "5h", used: 80, runs_out_at: "2026-11-12T14:20:00Z", resets_at: "2026-11-12T16:50:00Z" }), now)).toBe(
      `Claude 5 h runs out ≈ ${time("2026-11-12T14:20:00Z")} at this pace, before its reset at ${time("2026-11-12T16:50:00Z")}`,
    );
    expect(holdWords(offer(), now)).toBe("Claude's 5 h window is at 96 % until Nov 12 16:50");
  });
});

describe("the offers a device shows", () => {
  it("leaves out the ones seen here and keeps the newest seen", () => {
    expect(offersToShow([offer(), offer({ id: "a2" })], ["a1"]).map((o) => o.id)).toEqual(["a2"]);
    let seen: string[] = [];
    for (let i = 0; i < MAX_SEEN + 5; i++) seen = rememberOffer(seen, `o${i}`);
    expect(seen.length).toBe(MAX_SEEN);
    expect(seen[seen.length - 1]).toBe(`o${MAX_SEEN + 4}`);
    expect(rememberOffer(["a", "b"], "a")).toEqual(["b", "a"]);
  });
});
