import { afterEach, describe, expect, it } from "vitest";
import { setLang } from "@/lib/i18n";
import { clockWords, commandsWords, durationWords, filesWords, langOf, listWords, moneyWords, percentWords, sentenceOf, speakable, spokenName, timesWords, whenWords } from "./speakable";

afterEach(() => setLang("en"));

describe("the numbers said aloud", () => {
  it("counts times as a person does", () => {
    expect([1, 2, 3, 21].map(timesWords)).toEqual(["once", "twice", "3 times", "21 times"]);
    setLang("ru");
    expect([1, 2, 3, 5, 21, 22].map(timesWords)).toEqual(["один раз", "два раза", "3 раза", "5 раз", "21 раз", "22 раза"]);
  });

  it("says a stretch of work in minutes and hours, a single one without its number", () => {
    expect([20, 60, 12 * 60, 60 * 60, 70 * 60, 61 * 60, 125 * 60].map(durationWords)).toEqual(["", "a minute", "12 minutes", "an hour", "an hour and 10 minutes", "an hour and a minute", "2 hours and 5 minutes"]);
    setLang("ru");
    expect([60, 2 * 60, 5 * 60, 21 * 60, 60 * 60, 70 * 60, 3 * 3600].map(durationWords)).toEqual(["минуту", "2 минуты", "5 минут", "21 минуту", "час", "час и 10 минут", "3 часа"]);
  });

  it("rounds money the way it is said", () => {
    expect([0.12, 0.8, 1.2, 4.49, 14.6].map(moneyWords)).toEqual(["a few cents", "under a dollar", "about 1 dollar", "about 4 dollars", "about 15 dollars"]);
    setLang("ru");
    expect([0.12, 0.8, 1, 3.2, 21, 25].map(moneyWords)).toEqual(["несколько центов", "меньше доллара", "примерно 1 доллар", "примерно 3 доллара", "примерно 21 доллар", "примерно 25 долларов"]);
  });

  it("says a share and a list", () => {
    expect(percentWords(82.6)).toBe("82 percent");
    expect(listWords(["a.go"])).toBe("a.go");
    expect(listWords(["a.go", "b.go", "c.go"])).toBe("a.go, b.go and c.go");
    expect(filesWords(["/x/internal/a.go", "/x/web/b.ts", "/x/web/b.ts"])).toBe("a.go and b.ts");
    expect(filesWords(["/x/a.go", "/x/b.go", "/x/c.go", "/x/d.go"])).toBe("a.go, b.go and 2 more files");
    expect([1, 5].map(commandsWords)).toEqual(["one command", "5 commands"]);
    setLang("ru");
    expect([82, 1, 3].map(percentWords)).toEqual(["82 процента", "1 процент", "3 процента"]);
    expect(filesWords(["/a.go", "/b.go", "/c.go"])).toBe("a.go, b.go и ещё 1 файл");
    expect([1, 3, 5].map(commandsWords)).toEqual(["одну команду", "3 команды", "5 команд"]);
  });
});

describe("the time of day said aloud", () => {
  const at = (hour: number, minute = 0, day = 12) => new Date(2026, 10, day, hour, minute);

  it("says the hour in twelve in English and with its word in Russian", () => {
    expect([at(14), at(14, 30), at(9, 5), at(0), at(12)].map(clockWords)).toEqual(["2 PM", "2:30 PM", "9:05 AM", "midnight", "12 PM"]);
    setLang("ru");
    expect([at(14), at(14, 30), at(21), at(22), at(1), at(0)].map(clockWords)).toEqual(["14 часов", "14:30", "21 час", "22 часа", "1 час", "полночь"]);
  });

  it("says when a moment ahead comes: today, tomorrow, or a weekday", () => {
    const now = at(10);
    expect(whenWords(at(14), now)).toBe("at 2 PM");
    expect(whenWords(at(9, 0, 13), now)).toBe("tomorrow at 9 AM");
    expect(whenWords(at(14, 0, 16), now)).toBe("on Monday at 2 PM");
    setLang("ru");
    expect(whenWords(at(14), now)).toBe("в 14 часов");
    expect(whenWords(at(14, 0, 17), now)).toBe("во вторник в 14 часов");
    expect(whenWords(at(9, 30, 14), now)).toBe("в субботу в 9:30");
  });
});

describe("speakable", () => {
  it("reads a path as its file and a link as what it is", () => {
    expect(speakable("Fixed internal/server/roster.go and ~/Projects/app")).toBe("Fixed roster.go and app");
    expect(speakable("See https://github.com/example/app/pull/15.")).toBe("See pull request 15.");
    expect(speakable("Opened PR #15 and MR !9; closes #12")).toBe("Opened pull request 15 and merge request 9; closes number 12");
    expect(speakable("Docs at https://www.example.com/a/b")).toBe("Docs at example.com");
    expect(speakable("Issue https://gitlab.com/g/p/-/issues/7")).toBe("Issue issue 7");
  });

  it("keeps what only looks like a path: a fraction, a choice", () => {
    expect(speakable("Plan 3/7 done, Approve/Reject shown")).toBe("Plan 3/7 done, Approve/Reject shown");
  });

  it("leaves ids out and strips the marks", () => {
    expect(speakable("Committed a1b2c3d4 on w3:p1 for session 0f8fad5b-d9cb-469f-a165-70867728950e")).toBe("Committed on for session");
    expect(speakable("## **Done**: `make build` passes ✅ → ship it")).toBe("Done: make build passes, ship it");
    expect(speakable("- keep my_file.py & _private_")).toBe("keep my_file.py and private");
    expect(speakable("Before\n```go\nfunc main() {}\n```\nafter")).toBe("Before after");
  });

  it("ends a sentence, names a workspace and tells the language", () => {
    expect(sentenceOf("Tests pass")).toBe("Tests pass.");
    expect(sentenceOf("Ready?")).toBe("Ready?");
    expect(spokenName("remuda-podcast_two")).toBe("remuda podcast two");
    expect(langOf("Все тесты прошли")).toBe("ru");
    expect(langOf("Tests pass")).toBe("en");
  });

  it("speaks a Russian line in Russian words", () => {
    setLang("ru");
    expect(speakable("Готово: PR #15 и `make test` → зелёный")).toBe("Готово: пул-реквест 15 и make test, зелёный");
  });
});
