import { afterEach, describe, expect, it } from "vitest";
import { setLang } from "@/lib/i18n";
import type { Part } from "@/lib/speech";
import type { Item, Recap } from "./api";
import { dayScript, greeting, MAX_AGENTS, MAX_WORDS, recapScript, wordsOf } from "./spoken";

afterEach(() => setLang("en"));

const recap = (over: Partial<Recap> = {}): Recap => ({ prompts: 0, answers: 0, took: 0, tokens: 0, cost: 0, calls: 0, tools: {}, failed: 0, commands: [], edited: [], read: [], subagents: 0, questions: 0, asking: false, ...over });
const item = (pane: string, state: Item["state"], over: Partial<Recap> = {}, extra: Partial<Item> = {}): Item => ({ machine: "local", label: "Mac", workspace: "api", pane, dir: "/w/api", agent: "claude", state, recap: recap(over), ...extra });

const morning = new Date(2026, 10, 12, 9, 30).getTime();
const texts = (parts: Part[]) => parts.flatMap((part) => part.sentences.map((line) => line.text));
const keys = (parts: Part[]) => parts.map((part) => part.key);

describe("the day read aloud", () => {
  const items = [
    item("p1", "done", { prompts: 3, answers: 3, took: 14 * 60, cost: 4.2, edited: ["/w/api/internal/server/roster.go", "/w/api/web/src/app.ts"], commands: ["go test ./...", "make web"], said: "All **tests** pass and PR #15 is open." }),
    item("p2", "blocked", { prompts: 1, answers: 1, took: 50, asking: true, cost: 0.3 }, { agent: "codex", workspace: "web-app", machine: "gog", label: "GOG" }),
    item("p3", "idle", {}),
  ];

  it("greets, tells the ones waiting for you first, and closes with what is left", () => {
    const parts = dayScript({ items, period: "today", now: morning });
    expect(keys(parts)).toEqual(["intro", "gog|p2", "local|p1", "others", "closing"]);
    expect(texts(parts)).toEqual([
      "Good morning.",
      "Here is your day so far.",
      "2 agents did some work.",
      "Together they spent about 5 dollars at API prices.",
      "In web app on GOG, Codex is waiting for your answer.",
      "It answered once in a minute.",
      "In api, Claude Code has finished.",
      "It answered 3 times in 14 minutes.",
      "It edited roster.go and app.ts and ran 2 commands.",
      "That came to about 4 dollars.",
      "Its last answer:",
      "All tests pass and pull request 15 is open.",
      "One other agent had nothing to report.",
      "One agent still waits for you.",
      "That's all for now.",
    ]);
    expect(parts[2]!.title).toBe("api · Claude Code");
  });

  it("reads an agent's own words in their language's voice", () => {
    const said = dayScript({ items: [item("p1", "done", { answers: 1, said: "Готово, тесты зелёные" })], period: "today", now: morning });
    expect(said[1]!.sentences.at(-1)).toEqual({ text: "Готово, тесты зелёные.", lang: "ru" });
    expect(said[1]!.sentences[0]!.lang).toBe("en");
  });

  it("says a quiet day, and an empty fleet, in a line", () => {
    expect(texts(dayScript({ items: [item("p3", "idle")], period: "day", now: morning }))).toEqual(["Good morning.", "Here are the last 24 hours.", "It has been quiet: no agent did anything.", "That's all for now."]);
    expect(texts(dayScript({ items: [], period: "week", now: new Date(2026, 10, 12, 23, 30).getTime() }))).toEqual(["Hello.", "Here is the past week.", "No agent is running on a connected machine.", "That's all for now."]);
  });

  it("names a hidden folder's agent by the neutral name, without its files or its words", () => {
    const parts = dayScript({ items: [items[0]!], period: "today", now: morning, whoOf: () => ({ name: "Project A", hidden: true }) });
    expect(texts(parts).slice(4)).toEqual(["In Project A, Claude Code has finished.", "It answered 3 times in 14 minutes.", "It edited 2 files and ran 2 commands.", "That came to about 4 dollars.", "That's all for now."]);
  });

  it("swaps the hidden names out of every other agent's words", () => {
    const parts = dayScript({ items: [items[0]!], period: "today", now: morning, quote: (text) => text.replace("tests", "checks") });
    expect(texts(parts)).toContain("All checks pass and pull request 15 is open.");
  });

  it("says a watchdog note in words, its failures and a conversation it could not read", () => {
    const parts = dayScript({
      items: [
        item("p1", "working", { prompts: 2, answers: 1, took: 3 * 3600, failed: 2 }, { alert: "quiet for 25 min — working, but nothing written" }),
        item("p2", "working", {}, { error: "no such session" }),
      ],
      period: "today",
      now: morning,
    });
    expect(texts(parts)).toContain("In api, Claude Code needs a look.");
    expect(texts(parts)).toContain("It has gone quiet: working, but writing nothing.");
    expect(texts(parts)).toContain("It answered once in 3 hours.");
    expect(texts(parts)).toContain("2 calls failed along the way.");
    expect(texts(parts)).toContain("Its conversation could not be read.");
    expect(texts(parts).slice(-3)).toEqual(["One agent still waits for you.", "One agent is still at work.", "That's all for now."]);
  });

  it("tells the reviews waiting and the plan limits near their end", () => {
    const parts = dayScript({
      items: [items[0]!],
      period: "today",
      now: morning,
      reviews: [
        { name: "api", ready: true },
        { name: "web", ready: false },
        { name: "docs", ready: false },
      ],
      limits: [
        { agent: "claude", name: "5h", used: 86.4, resets_at: new Date(2026, 10, 12, 14).toISOString(), at: "" },
        { agent: "claude", name: "week", used: 100, resets_at: new Date(2026, 10, 16, 9).toISOString(), at: "" },
        { agent: "codex", name: "week", used: 31, resets_at: new Date(2026, 10, 16, 9).toISOString(), at: "" },
      ],
    });
    expect(keys(parts)).toEqual(["intro", "local|p1", "review", "limits", "closing"]);
    expect(parts[2]!.sentences.map((line) => line.text)).toEqual(["In api, the agent says the work is ready for another look.", "2 more workspaces have changes waiting for your review."]);
    expect(parts[3]!.sentences.map((line) => line.text)).toEqual(["Claude's weekly limit is used up; it resets on Monday at 9 AM.", "Claude's 5-hour limit is 86 percent used; it resets at 2 PM."]);
  });

  it("stays within a few minutes: the agents past the length counted in one line", () => {
    const many = Array.from({ length: 40 }, (_, i) => item(`p${i}`, "done", { prompts: 4, answers: 4, took: 600, cost: 2, edited: ["/w/a.go", "/w/b.go", "/w/c.go"], commands: ["make"], failed: 1, said: "Everything is in place and the tests pass on every machine we have." }, { workspace: `ws${i}` }));
    const parts = dayScript({ items: many, period: "today", now: morning });
    const told = parts.filter((part) => part.key.startsWith("local|"));
    expect(told.length).toBeLessThanOrEqual(MAX_AGENTS);
    expect(parts.reduce((sum, part) => sum + wordsOf(part), 0)).toBeLessThanOrEqual(MAX_WORDS);
    expect(texts(parts)).toContain(`${40 - told.length} more agents also worked; the panel lists them.`);
  });

  it("greets by the hour", () => {
    expect([6, 13, 19, 2].map(greeting)).toEqual(["Good morning.", "Good afternoon.", "Good evening.", "Hello."]);
  });

  it("reads naturally in Russian: the counts in their forms, the times and the money as said", () => {
    setLang("ru");
    const parts = dayScript({
      items,
      period: "today",
      now: morning,
      limits: [{ agent: "claude", name: "5h", used: 82, resets_at: new Date(2026, 10, 12, 14).toISOString(), at: "" }],
    });
    expect(texts(parts)).toEqual([
      "Доброе утро.",
      "Вот как прошёл ваш день.",
      "2 агента поработали.",
      "Вместе они потратили примерно 5 долларов по ценам API.",
      "В web app на GOG Codex ждёт вашего ответа.",
      "Он ответил один раз за минуту.",
      "В api Claude Code закончил работу.",
      "Он ответил 3 раза за 14 минут.",
      "Он изменил roster.go и app.ts и запустил 2 команды.",
      "Это стоило примерно 4 доллара.",
      "Последний ответ:",
      "All tests pass and пул-реквест 15 is open.",
      "Ещё 1 агент ничего не делал.",
      "Лимит Claude на 5 часов использован на 82 процента; он сбросится в 14 часов.",
      "1 агент всё ещё ждёт вас.",
      "На этом пока всё.",
    ]);
    expect(parts.find((part) => part.key === "local|p1")!.sentences.at(-1)!.lang).toBe("en");
  });
});

describe("a conversation's recap read aloud", () => {
  it("is a part a line, so skipping moves a line at a time", () => {
    const parts = recapScript("remuda-podcast", recap({ prompts: 5, answers: 4, took: 40 * 60, cost: 2.4, commands: ["make test"], asking: true, said: "Which port should it use?" }));
    expect(parts.map((part) => part.sentences.map((line) => line.text))).toEqual([
      ["Here is what the agent in remuda podcast did."],
      ["It answered 4 times in 40 minutes."],
      ["It ran one command."],
      ["It spent about 2 dollars at API prices."],
      ["A question is open."],
      ["Its last answer:", "Which port should it use?"],
      ["That's all for now."],
    ]);
    expect(parts.every((part) => part.title === "remuda-podcast")).toBe(true);
  });

  it("says when there is nothing yet", () => {
    expect(texts(recapScript("api", recap()))).toEqual(["Here is what the agent in api did.", "Nothing in this conversation yet.", "That's all for now."]);
  });
});
