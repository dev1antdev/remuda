// The page in the person's language. t() looks an English string up in the dictionary of the language chosen
// (system, en, ru) and answers the English itself when there is no translation, so nothing ever shows a gap; {name}
// placeholders are filled from vars; a form table picks its plural by the language's rule. The language is module
// state read at every call, so a pure module's words (the radar's, the budgets', the time words) follow it too.

import { ru } from "./i18n/ru";

export type Lang = "en" | "ru";
export type LangPref = "system" | Lang;
export const LANG_KEY = "lang";
export const LANGS: Lang[] = ["en", "ru"];
export const LANG_NAMES: Record<Lang, string> = { en: "English", ru: "Русский" };

// Forms is a translation: one string, or the plural forms of a counted phrase in the language's order
export type Forms = string | string[];
export type Dictionary = Record<string, Forms>;
const DICTS: Record<Lang, Dictionary> = { en: {}, ru };

let lang: Lang = "en";

export const currentLang = (): Lang => lang;

export function setLang(next: Lang): void {
  lang = next;
}

// langPrefOf keeps a stored pref only when it names a choice
export const langPrefOf = (value: string): LangPref => (value === "ru" || value === "en" ? value : "system");

// languageOf resolves a pref: system means the first of the browser's languages the page speaks, else English
export function languageOf(pref: string, languages: readonly string[] = []): Lang {
  if (pref === "ru" || pref === "en") return pref;
  for (const tag of languages) {
    const code = tag.toLowerCase().split("-")[0];
    if (code === "ru" || code === "en") return code;
  }
  return "en";
}

export type Vars = Record<string, string | number>;

// t is the string in the current language with its placeholders filled; a form table picks its form by vars.n
export function t(text: string, vars?: Vars): string {
  const entry = DICTS[lang][text];
  const out = Array.isArray(entry) ? pick(entry, Number(vars?.n ?? 0)) : (entry ?? text);
  return vars ? fill(out, vars) : out;
}

// tn counts: the English pair by the count ("{n} file" / "{n} files"), another language's form table under the plural key
export function tn(n: number, one: string, many: string, vars?: Vars): string {
  const entry = DICTS[lang][many];
  const out = Array.isArray(entry) ? pick(entry, n) : typeof entry === "string" ? entry : n === 1 ? one : many;
  return fill(out, { n, ...vars });
}

// fill puts the vars into their {name} slots; a slot without a var stays as written
export function fill(text: string, vars: Vars): string {
  return text.replace(/\{(\w+)\}/g, (slot, name: string) => (name in vars ? String(vars[name]) : slot));
}

// pluralIndex is the form a count takes: English one / other; Russian one (1, 21, 31…), few (2–4, 22–24…), many (0, 5–20, 25–30…)
export function pluralIndex(n: number, language: Lang = lang): number {
  const a = Math.abs(Math.trunc(n));
  if (language === "ru") {
    const m10 = a % 10;
    const m100 = a % 100;
    if (m10 === 1 && m100 !== 11) return 0;
    if (m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14)) return 1;
    return 2;
  }
  return a === 1 ? 0 : 1;
}

function pick(forms: string[], n: number): string {
  return forms[Math.min(pluralIndex(n), forms.length - 1)] ?? forms[0] ?? "";
}

// locale is what the Intl formats take: the language's tag, or the browser's own for English
export const locale = (): string | undefined => (lang === "ru" ? "ru-RU" : undefined);

// hasTranslation says whether the language knows a string (the dictionary's own test uses it)
export const hasTranslation = (text: string, language: Lang = lang): boolean => text in DICTS[language];
