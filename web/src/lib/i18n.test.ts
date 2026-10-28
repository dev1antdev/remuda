import { afterEach, describe, expect, it } from "vitest";
import { currentLang, fill, hasTranslation, languageOf, langPrefOf, locale, pluralIndex, setLang, t, tn } from "./i18n";

afterEach(() => setLang("en"));

describe("t", () => {
  it("answers the English itself when the language is English or the string is unknown", () => {
    expect(t("Mark reviewed")).toBe("Mark reviewed");
    setLang("ru");
    expect(t("Settings")).toBe("Настройки");
    expect(t("A string nobody translated")).toBe("A string nobody translated");
  });
  it("fills the placeholders, in both languages, and leaves a slot without a var as written", () => {
    expect(t("New tab in {workspace}…", { workspace: "remuda" })).toBe("New tab in remuda…");
    expect(fill("{a} and {b}", { a: 1 })).toBe("1 and {b}");
    expect(fill("{a}", { a: undefined })).toBe("");
    setLang("ru");
    expect(t("New tab in {workspace}…", { workspace: "remuda" })).toBe("Новая вкладка в remuda…");
  });
  it("picks the plural form of a form table by n", () => {
    setLang("ru");
    expect(t("{n} tabs", { n: 1 })).toBe("1 вкладка");
    expect(t("{n} tabs", { n: 3 })).toBe("3 вкладки");
    expect(t("{n} tabs", { n: 5 })).toBe("5 вкладок");
  });
});

describe("tn", () => {
  it("counts in English by one and many", () => {
    expect(tn(1, "{n} file", "{n} files")).toBe("1 file");
    expect(tn(2, "{n} file", "{n} files")).toBe("2 files");
    expect(tn(0, "{n} file", "{n} files")).toBe("0 files");
  });
  it("counts in Russian by the form table under the plural key, with the other vars filled", () => {
    setLang("ru");
    expect(tn(1, "{n} file", "{n} files")).toBe("1 файл");
    expect(tn(21, "{n} file", "{n} files")).toBe("21 файл");
    expect(tn(2, "{n} file", "{n} files")).toBe("2 файла");
    expect(tn(11, "{n} file", "{n} files")).toBe("11 файлов");
    expect(tn(2, "Close “{name}”? {n} pane closes.", "Close “{name}”? {n} panes close and the agents in them stop.", { name: "x" })).toBe("Закрыть «x»? Закроются 2 панели, агенты в них остановятся.");
  });
  it("falls back to the English pair for a phrase the language lacks", () => {
    setLang("ru");
    expect(tn(3, "{n} widget", "{n} widgets")).toBe("3 widgets");
  });
});

describe("pluralIndex", () => {
  it("is one / other in English", () => {
    expect([0, 1, 2, 5, 21].map((n) => pluralIndex(n, "en"))).toEqual([1, 0, 1, 1, 1]);
  });
  it("is one / few / many in Russian, the teens many", () => {
    expect([1, 21, 101].map((n) => pluralIndex(n, "ru"))).toEqual([0, 0, 0]);
    expect([2, 3, 4, 22, 24].map((n) => pluralIndex(n, "ru"))).toEqual([1, 1, 1, 1, 1]);
    expect([0, 5, 11, 12, 14, 19, 20, 25, 111].map((n) => pluralIndex(n, "ru"))).toEqual([2, 2, 2, 2, 2, 2, 2, 2, 2]);
    expect(pluralIndex(-3, "ru")).toBe(1);
  });
});

describe("the language", () => {
  it("resolves the pref: a named language, else the first of the browser's the page speaks, else English", () => {
    expect(languageOf("ru")).toBe("ru");
    expect(languageOf("en", ["ru"])).toBe("en");
    expect(languageOf("system", ["de-DE", "ru-RU", "en"])).toBe("ru");
    expect(languageOf("system", ["de", "fr"])).toBe("en");
    expect(languageOf("system")).toBe("en");
    expect(languageOf("nonsense", ["ru"])).toBe("ru");
  });
  it("keeps a stored pref only when it names a choice", () => {
    expect(langPrefOf("ru")).toBe("ru");
    expect(langPrefOf("en")).toBe("en");
    expect(langPrefOf("klingon")).toBe("system");
  });
  it("is module state the time words and the Intl formats read", () => {
    expect(currentLang()).toBe("en");
    expect(locale()).toBeUndefined();
    setLang("ru");
    expect(currentLang()).toBe("ru");
    expect(locale()).toBe("ru-RU");
    expect(hasTranslation("Settings")).toBe(true);
    expect(hasTranslation("Settings", "en")).toBe(false);
  });
});
