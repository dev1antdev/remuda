// The words a voice says, as a person says them in the page's language: how many times, how long, how much money, what
// time, what share of a limit; and a line of an agent's own text made speakable — its Markdown marks, paths, links and
// ids turned into words or left out. The pure part of reading the day aloud.

import { currentLang, t, tn } from "@/lib/i18n";

// timesWords: "once", "twice", "5 times"
export function timesWords(n: number): string {
  if (n === 1) return t("once");
  if (n === 2) return t("twice");
  return tn(n, "{n} time", "{n} times");
}

// durationWords is a stretch of work in whole minutes, as said after "in": "a minute", "12 minutes", "an hour and 10
// minutes", "2 hours"; under half a minute is ""
export function durationWords(seconds: number): string {
  const minutes = Math.round(seconds / 60);
  if (minutes < 1) return "";
  if (minutes < 60) return minutes === 1 ? t("a minute") : tn(minutes, "{n} minute", "{n} minutes");
  const hours = Math.floor(minutes / 60);
  const rest = minutes % 60;
  const hourWords = hours === 1 ? t("an hour") : tn(hours, "{n} hour", "{n} hours");
  if (rest === 0) return hourWords;
  return t("{hours} and {minutes}", { hours: hourWords, minutes: rest === 1 ? t("a minute") : tn(rest, "{n} minute", "{n} minutes") });
}

// moneyWords is a sum at API prices, rounded the way a person rounds it aloud: "a few cents", "under a dollar",
// "about 4 dollars"
export function moneyWords(usd: number): string {
  if (usd < 0.5) return t("a few cents");
  if (usd < 1) return t("under a dollar");
  const dollars = Math.round(usd);
  return t("about {amount}", { amount: tn(dollars, "{n} dollar", "{n} dollars") });
}

// percentWords: "82 percent"
export const percentWords = (n: number): string => tn(Math.floor(n), "{n} percent", "{n} percent");

// clockWords is a time of day as it is said: "2 PM", "2:30 PM" and "midnight" in English; "14 часов", "14:30" and
// "полночь" in Russian, whose voices read a whole hour better with its word
export function clockWords(at: Date): string {
  const hour = at.getHours();
  const minute = at.getMinutes();
  if (hour === 0 && minute === 0) return t("midnight");
  const mm = String(minute).padStart(2, "0");
  if (currentLang() === "ru") return minute ? `${hour}:${mm}` : tn(hour, "{n} o'clock", "{n} o'clock");
  const h12 = hour % 12 || 12;
  const half = hour < 12 ? "AM" : "PM";
  return minute ? `${h12}:${mm} ${half}` : `${h12} ${half}`;
}

// ON_DAY says a weekday ahead and its time, by the weekday's number (Sunday first, as Date counts)
const ON_DAY: ((time: string) => string)[] = [
  (time) => t("on Sunday at {time}", { time }),
  (time) => t("on Monday at {time}", { time }),
  (time) => t("on Tuesday at {time}", { time }),
  (time) => t("on Wednesday at {time}", { time }),
  (time) => t("on Thursday at {time}", { time }),
  (time) => t("on Friday at {time}", { time }),
  (time) => t("on Saturday at {time}", { time }),
];

// whenWords is a moment ahead as said after "resets": "at 2 PM" today, "tomorrow at 9 AM", "on Friday at 2 PM"
export function whenWords(at: Date, now: Date): string {
  const time = clockWords(at);
  const day = (date: Date) => new Date(date.getFullYear(), date.getMonth(), date.getDate()).getTime();
  const days = Math.round((day(at) - day(now)) / 86400_000);
  if (days <= 0) return t("at {time}", { time });
  if (days === 1) return t("tomorrow at {time}", { time });
  return ON_DAY[at.getDay()]!(time);
}

// listWords joins names as a sentence lists them: "a", "a and b", "a, b and c"
export function listWords(names: string[]): string {
  if (names.length <= 1) return names[0] ?? "";
  return t("{list} and {last}", { list: names.slice(0, -1).join(", "), last: names[names.length - 1]! });
}

// langOf is the language a text is in, as far as a voice cares: Russian for Cyrillic, else English
export const langOf = (text: string): string => (/[Ѐ-ӿ]/.test(text) ? "ru" : "en");

// spokenName is a workspace's or a file's name the way it is said: its dashes and underscores as spaces
export const spokenName = (name: string): string => name.replace(/[-_]+/g, " ").replace(/\s+/g, " ").trim();

const URL_RE = /\bhttps?:\/\/[^\s<>()"'`]+/g;
const UUID_RE = /\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b/gi;
const HASH_RE = /\b(?=[0-9a-f]*\d)(?=[0-9a-f]*[a-f])[0-9a-f]{7,40}\b/gi;
const PANE_RE = /\bw\d+:[pt]\d+\b/g;
const PATH_RE = /(?<![\w.:/@-])(?:~|\.{1,2})?\/?(?:[\w@.+-]+\/)+[\w@.+-]*/g;
const PICTURES_RE = /\p{Extended_Pictographic}️?/gu;

// linkWords is a link as it is said: a pull request, a merge request or an issue by its number, else its site
function linkWords(url: string): string {
  const trimmed = url.replace(/[.,;:!?]+$/, "");
  const tail = url.slice(trimmed.length);
  const pull = /\/pull\/(\d+)/.exec(trimmed);
  if (pull) return t("pull request {n}", { n: pull[1] }) + tail;
  const merge = /\/merge_requests\/(\d+)/.exec(trimmed);
  if (merge) return t("merge request {n}", { n: merge[1] }) + tail;
  const issue = /\/issues\/(\d+)/.exec(trimmed);
  if (issue) return t("issue {n}", { n: issue[1] }) + tail;
  try {
    return new URL(trimmed).hostname.replace(/^www\./, "") + tail;
  } catch {
    return tail;
  }
}

// pathWords is a path as it is said: its last part, when it is surely a path — a file with an extension, a path from
// the root or home, or three parts and more; "3/7" or "Approve/Reject" stay as they are
function pathWords(token: string): string {
  const parts = token.split("/").filter(Boolean);
  const last = parts[parts.length - 1] ?? "";
  if (parts.every((part) => /^\d+$/.test(part))) return token;
  const rooted = /^(~|\.{1,2})?\//.test(token);
  if (rooted || /\.[a-z0-9]{1,8}$/i.test(last) || parts.length >= 3) return last;
  return token;
}

// speakable is a line of an agent's text as a voice should read it: code fences and Markdown marks gone, a link as a
// pull request's number or its site, a path as its file, a commit's hash, a session's id and a pane's id left out, a
// number after # said as one, pictographs dropped, arrows read as a pause
export function speakable(text: string): string {
  let out = text.replace(/```[\s\S]*?(```|$)/g, " ");
  out = out.replace(URL_RE, linkWords);
  out = out.replace(/\bPR\s*#(\d+)/g, (_, n: string) => t("pull request {n}", { n }));
  out = out.replace(/\bMR\s*!(\d+)/g, (_, n: string) => t("merge request {n}", { n }));
  out = out.replace(/(^|[\s(])#(\d+)\b/g, (_, before: string, n: string) => `${before}${t("number {n}", { n })}`);
  out = out.replace(UUID_RE, " ").replace(HASH_RE, " ").replace(PANE_RE, " ");
  out = out.replace(PATH_RE, pathWords);
  out = out.replace(/^\s*(#{1,6}|>|[-*+]|\d+[.)])\s+/, "");
  out = out.replace(/\*\*|__|[`*~]/g, "").replace(/(^|\s)_+|_+(\s|$)/g, "$1$2");
  out = out.replace(PICTURES_RE, " ");
  out = out.replace(/\s*(→|->|=>|⇒)\s*/g, ", ");
  out = out.replace(/\s+&\s+/g, ` ${t("and")} `);
  out = out.replace(/\s+/g, " ").replace(/\s+([,.;:!?])/g, "$1").replace(/([,;:])(?=[,;:.!?])/g, "").replace(/\(\s*\)/g, "");
  return out.replace(/^[\s,.;:]+/, "").trim();
}

// sentenceOf ends a text as a sentence: a full stop when it has no mark of its own
export const sentenceOf = (text: string): string => (/[.!?…:]$/.test(text) ? text : `${text}.`);

// filesWords names the files a sentence lists by their own names: "store.ts", "a.go and b.go", "a.go, b.go and 3 more files"
export function filesWords(paths: string[]): string {
  const names = [...new Set(paths.map((path) => path.split("/").filter(Boolean).pop() ?? path))];
  if (names.length <= 2) return listWords(names);
  return listWords([names[0]!, names[1]!, tn(names.length - 2, "{n} more file", "{n} more files")]);
}

// commandsWords: "one command", "5 commands"
export const commandsWords = (n: number): string => (n === 1 ? t("one command") : tn(n, "{n} command", "{n} commands"));
