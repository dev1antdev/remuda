import { afterEach, describe, expect, it } from "vitest";
import type { Recap } from "@/features/standup/api";
import { setLang } from "@/lib/i18n";
import { creatureTalk, speechOf } from "./zooTalk";

afterEach(() => setLang("en"));

const recap = (over: Partial<Recap> = {}): Recap => ({ prompts: 0, answers: 0, took: 0, tokens: 0, cost: 0, calls: 0, tools: {}, failed: 0, commands: [], edited: [], read: [], subagents: 0, questions: 0, asking: false, ...over });
const words = (lines: { text: string }[]) => lines.map((line) => line.text);

describe("what a creature says of its day", () => {
  it("tells what it did and how it stands, in a sentence or two", () => {
    expect(words(creatureTalk(recap({ prompts: 2, answers: 2, edited: ["/w/internal/a.go"] }), "busy"))).toEqual(["Today I answered twice and edited a.go.", "I'm still at it."]);
    expect(words(creatureTalk(recap({ prompts: 3, answers: 3, commands: ["make", "make test"] }), "asking"))).toEqual(["Today I answered 3 times and ran 2 commands.", "Now I'm waiting for your answer."]);
    expect(words(creatureTalk(recap({ prompts: 1 }), "worried"))).toEqual(["Today I'm still on my first answer.", "Something is off here: have a look."]);
    expect(words(creatureTalk(recap({ calls: 4 }), "drained"))).toEqual(["Today I've been busy with my tools.", "I've hit my plan limit and wait for it to reset."]);
  });

  it("quotes its last answer once it rests or has finished, in the answer's own language", () => {
    const lines = creatureTalk(recap({ prompts: 1, answers: 1 }), "dancing", "Готово: PR #15 открыт");
    expect(lines).toEqual([
      { text: "Today I answered once.", lang: "en" },
      { text: "My last answer:", lang: "en" },
      { text: "Готово: pull request 15 открыт.", lang: "ru" },
    ]);
    expect(words(creatureTalk(recap({ answers: 1 }), "dancing"))).toEqual(["Today I answered once.", "I've just finished."]);
    expect(words(creatureTalk(null, "asleep"))).toEqual(["I haven't done anything yet today.", "Now I'm resting."]);
    expect(words(creatureTalk(recap(), "puzzled"))).toEqual(["I haven't done anything yet today."]);
  });

  it("speaks Russian on a Russian page", () => {
    setLang("ru");
    expect(words(creatureTalk(recap({ answers: 5, edited: ["/a.go", "/b.go", "/c.go", "/d.go"] }), "busy"))).toEqual(["Сегодня я ответил 5 раз и изменил a.go, b.go и ещё 2 файла.", "Я ещё работаю."]);
  });
});

describe("speechOf", () => {
  it("gives each species the pitch of its chirp and a pace of its own", () => {
    expect(speechOf("claude")).toEqual({ pitch: 1.26, rate: 1.04 });
    expect(speechOf("codex")).toEqual({ pitch: 0.75, rate: 0.92 });
    expect(speechOf("gemini")).toEqual({ pitch: 1.5, rate: 1.04 });
    expect(speechOf("kimi")).toEqual({ pitch: 1, rate: 1.08 });
    expect(speechOf("other")).toEqual({ pitch: 0.63, rate: 0.9 });
  });
});
