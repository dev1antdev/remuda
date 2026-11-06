// The language the page speaks: the pref `lang` (system, en, ru) read before anything renders — this module is main.tsx's
// first import, so every table a module builds at load is already in the language — and put on the document's root. A
// change of the pref writes it and reloads the page: the modules' tables, the terminal engines and every pure module's
// words come back in the new language at once, which a re-render could not promise.

import { LANG_KEY, langPrefOf, languageOf, setLang, type Lang, type LangPref } from "@/lib/i18n";
import { readPref, writePref } from "@/lib/prefs";

export const readLangPref = (): LangPref => langPrefOf(readPref<string>(LANG_KEY, "system"));

// startLang sets the language from the pref and the browser's own before the first render
export function startLang(): Lang {
  const language = languageOf(readLangPref(), typeof navigator === "undefined" ? [] : navigator.languages);
  setLang(language);
  if (typeof document !== "undefined") document.documentElement.lang = language;
  return language;
}

// setLangPref keeps the choice and starts the page over in it
export function setLangPref(pref: LangPref): void {
  if (pref === readLangPref()) return;
  writePref(LANG_KEY, pref);
  location.reload();
}

startLang();
