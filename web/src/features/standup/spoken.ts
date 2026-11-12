// The day read aloud: the standup — with the reviews waiting and the plan limits near their end, when given — as a
// short script in the page's language, told the way a person tells it: a greeting, who did what (the ones waiting for
// you first), what it cost, what is left, a closing line. A part per agent, so the player skips an agent at a time; at
// most MAX_WORDS, about four minutes, the agents past it counted in one line. One conversation's recap reads the same
// way. The pure part, tested.

import type { LimitWindow } from "@/features/roster/api";
import { agentName } from "@/features/usage/limits";
import { currentLang, t, tn } from "@/lib/i18n";
import type { Part, Sentence } from "@/lib/speech";
import { VENDOR_NAME } from "@/ui/brand";
import type { Item, Recap } from "./api";
import { commandsWords, durationWords, filesWords, langOf, moneyWords, percentWords, sentenceOf, speakable, spokenName, timesWords, whenWords } from "./speakable";
import { order, quiet, type Period } from "./standup";

// MAX_WORDS is the script's length at most: about four minutes at a voice's usual pace of 150 words a minute
export const MAX_WORDS = 600;
// MAX_AGENTS is how many agents the script tells of one by one
export const MAX_AGENTS = 12;
// LIMIT_FROM is the share of a plan window from which the script mentions it
const LIMIT_FROM = 80;

// Who is an agent's place as it is said; hidden: its folder is hidden under the screen-sharing mode, so its files and words are not said
export type Who = { name: string; hidden: boolean };
// ReviewWaiting is a workspace whose changes wait for a look: ready, rejected at the last look and said to be ready again
export type ReviewWaiting = { name: string; ready: boolean };

export type DayInput = {
  items: Item[];
  period: Period;
  now: number;
  whoOf?: (item: Item) => Who;
  quote?: (text: string) => string; // what an agent said, as it may be said aloud (the screen-sharing mode's names swapped)
  reviews?: ReviewWaiting[];
  limits?: LimitWindow[];
};

export const say = (text: string): Sentence => ({ text, lang: currentLang() });
// quoted is an agent's own words as a sentence, in the voice of their own language
export const quoted = (text: string): Sentence => ({ text: sentenceOf(speakable(text)), lang: langOf(text) });

export const wordsOf = (part: Part): number => part.sentences.reduce((sum, line) => sum + line.text.split(/\s+/).filter(Boolean).length, 0);

// greeting is the hour's hello
export function greeting(hour: number): string {
  if (hour >= 5 && hour < 12) return t("Good morning.");
  if (hour >= 12 && hour < 18) return t("Good afternoon.");
  if (hour >= 18 && hour < 23) return t("Good evening.");
  return t("Hello.");
}

const periodLine = (period: Period): string => (period === "day" ? t("Here are the last 24 hours.") : period === "week" ? t("Here is the past week.") : t("Here is your day so far."));

// agentWords is the agent's kind as it is called: "Claude Code", "Codex"
export const agentWords = (agent: string): string => VENDOR_NAME[agent] ?? (agent || t("an agent"));

const defaultWho = (item: Item): Who => ({
  name: item.machine !== "local" && item.label ? t("{workspace} on {machine}", { workspace: spokenName(item.workspace), machine: spokenName(item.label) }) : spokenName(item.workspace),
  hidden: false,
});

const asks = (item: Item) => Boolean(item.alert) || item.state === "blocked";

// alertWords is a watchdog note said in the page's language, the server's English kept only for a note it does not know
function alertWords(alert: string): Sentence {
  if (alert.startsWith("quiet for")) return say(t("It has gone quiet: working, but writing nothing."));
  if (alert.startsWith("looping")) return say(t("It seems to go round in circles, repeating the same call."));
  if (alert.startsWith("waiting for your answer")) return say(t("It has been waiting for your answer for a while."));
  return quoted(alert);
}

function stateSentence(item: Item, agent: string, where: string): string {
  if (item.alert) return t("In {where}, {agent} needs a look.", { where, agent });
  if (item.state === "blocked") return t("In {where}, {agent} is waiting for your answer.", { where, agent });
  if (item.state === "done") return t("In {where}, {agent} has finished.", { where, agent });
  if (item.state === "working") return t("In {where}, {agent} is still at work.", { where, agent });
  if (item.state === "idle") return t("In {where}, {agent} is resting.", { where, agent });
  return t("In {where}, {agent} did some work.", { where, agent });
}

// workSentence is how much it answered and how long that took
export function workSentence(r: Recap): string {
  if (r.answers > 0) {
    const time = durationWords(r.took);
    return time ? t("It answered {times} in {time}.", { times: timesWords(r.answers), time }) : t("It answered {times}.", { times: timesWords(r.answers) });
  }
  if (r.prompts > 0) return t("It has not answered yet.");
  return tn(r.calls, "It made one tool call.", "It made {n} tool calls.");
}

// ranOf is how many shell commands it ran: the recap lists the first twenty, its tools count them all
export const ranOf = (r: Recap): number => Math.max(r.commands.length, r.tools?.Bash ?? 0);

// didSentence is what it edited and ran; a hidden folder's files go by their count
export function didSentence(r: Recap, hidden: boolean): string {
  const files = r.edited.length === 0 ? "" : hidden ? tn(r.edited.length, "{n} file", "{n} files") : filesWords(r.edited);
  const ran = ranOf(r);
  if (files && ran) return t("It edited {files} and ran {commands}.", { files, commands: commandsWords(ran) });
  if (files) return t("It edited {files}.", { files });
  if (ran) return t("It ran {commands}.", { commands: commandsWords(ran) });
  return "";
}

function agentPart(item: Item, who: Who, quote: (text: string) => string): Part {
  const agent = agentWords(item.agent);
  const sentences = [say(stateSentence(item, agent, who.name))];
  if (item.alert && !who.hidden) sentences.push(alertWords(item.alert));
  if (item.error) sentences.push(say(t("Its conversation could not be read.")));
  else {
    const r = item.recap;
    sentences.push(say(workSentence(r)));
    const did = didSentence(r, who.hidden);
    if (did) sentences.push(say(did));
    if (r.failed > 0) sentences.push(say(tn(r.failed, "One call failed along the way.", "{n} calls failed along the way.")));
    if (r.cost >= 1) sentences.push(say(t("That came to {money}.", { money: moneyWords(r.cost) })));
    if (r.said && !who.hidden) sentences.push(say(t("Its last answer:")), quoted(quote(r.said)));
  }
  return { key: `${item.machine}|${item.pane}`, title: `${who.name} · ${agent}`, sentences };
}

function reviewPart(reviews: ReviewWaiting[]): Part | null {
  if (reviews.length === 0) return null;
  const ready = reviews.filter((review) => review.ready).slice(0, 2);
  const sentences = ready.map((review) => say(t("In {name}, the agent says the work is ready for another look.", { name: review.name })));
  const rest = reviews.length - ready.length;
  if (rest > 0) sentences.push(say(ready.length ? tn(rest, "One more workspace has changes waiting for your review.", "{n} more workspaces have changes waiting for your review.") : tn(rest, "One workspace has changes waiting for your review.", "{n} workspaces have changes waiting for your review.")));
  return { key: "review", title: t("Review"), sentences };
}

function limitSentence(w: LimitWindow, now: number): string {
  const vars = { agent: agentName(w.agent), pct: percentWords(w.used), when: whenWords(new Date(w.resets_at), new Date(now)) };
  if (w.name === "week") return w.used >= 100 ? t("{agent}'s weekly limit is used up; it resets {when}.", vars) : t("{agent}'s weekly limit is {pct} used; it resets {when}.", vars);
  return w.used >= 100 ? t("{agent}'s 5-hour limit is used up; it resets {when}.", vars) : t("{agent}'s 5-hour limit is {pct} used; it resets {when}.", vars);
}

function limitsPart(limits: LimitWindow[], now: number): Part | null {
  const near = limits
    .filter((w) => (w.name === "5h" || w.name === "week") && w.used >= LIMIT_FROM && Date.parse(w.resets_at) > now)
    .sort((a, b) => b.used - a.used)
    .slice(0, 3);
  return near.length ? { key: "limits", title: t("Plan limits"), sentences: near.map((w) => say(limitSentence(w, now))) } : null;
}

function closingPart(items: Item[]): Part {
  const waiting = items.filter(asks).length;
  const working = items.filter((item) => !asks(item) && item.state === "working").length;
  const sentences: Sentence[] = [];
  if (waiting) sentences.push(say(tn(waiting, "One agent still waits for you.", "{n} agents still wait for you.")));
  if (working) sentences.push(say(tn(working, "One agent is still at work.", "{n} agents are still at work.")));
  sentences.push(say(t("That's all for now.")));
  return { key: "closing", title: t("What is left"), sentences };
}

// dayScript is the standup as it is read aloud
export function dayScript(input: DayInput): Part[] {
  const { items, period, now } = input;
  const whoOf = input.whoOf ?? defaultWho;
  const quote = input.quote ?? ((text: string) => text);
  const busy = order(items.filter((item) => item.error || !quiet(item.recap)));
  const idle = items.length - busy.length;
  const opening = [say(greeting(new Date(now).getHours())), say(periodLine(period))];
  if (items.length === 0) opening.push(say(t("No agent is running on a connected machine.")));
  else if (busy.length === 0) opening.push(say(t("It has been quiet: no agent did anything.")));
  else {
    opening.push(say(tn(busy.length, "One agent did some work.", "{n} agents did some work.")));
    const cost = busy.reduce((sum, item) => sum + (item.recap.cost || 0), 0);
    if (cost >= 0.01) opening.push(say(busy.length === 1 ? t("It spent {money} at API prices.", { money: moneyWords(cost) }) : t("Together they spent {money} at API prices.", { money: moneyWords(cost) })));
  }
  const tail = [reviewPart(input.reviews ?? []), limitsPart(input.limits ?? [], now), closingPart(items)].filter((part): part is Part => part !== null);
  const head: Part = { key: "intro", title: t("Standup"), sentences: opening };
  let room = MAX_WORDS - wordsOf(head) - tail.reduce((sum, part) => sum + wordsOf(part), 0) - 20; // 20: the line for the others
  const told: Part[] = [];
  for (const item of busy) {
    const part = agentPart(item, whoOf(item), quote);
    if (told.length > 0 && (told.length >= MAX_AGENTS || wordsOf(part) > room)) break;
    told.push(part);
    room -= wordsOf(part);
  }
  const more = busy.length - told.length;
  const others: Sentence[] = [];
  if (more > 0) others.push(say(tn(more, "One more agent also worked; the panel lists it.", "{n} more agents also worked; the panel lists them.")));
  if (idle > 0 && busy.length > 0) others.push(say(tn(idle, "One other agent had nothing to report.", "{n} other agents had nothing to report.")));
  const rest: Part[] = others.length ? [{ key: "others", title: t("The others"), sentences: others }] : [];
  return [head, ...told, ...rest, ...tail];
}

// recapScript is one conversation's recap read aloud, a part per thing it says, so skipping moves a line at a time
export function recapScript(title: string, recap: Recap, quote: (text: string) => string = (text) => text): Part[] {
  const groups: Sentence[][] = [[say(t("Here is what the agent in {title} did.", { title: spokenName(title) }))]];
  if (quiet(recap)) groups.push([say(t("Nothing in this conversation yet."))]);
  else {
    groups.push([say(workSentence(recap))]);
    const did = didSentence(recap, false);
    if (did) groups.push([say(did)]);
    if (recap.failed > 0) groups.push([say(tn(recap.failed, "One call failed along the way.", "{n} calls failed along the way."))]);
    if (recap.cost >= 0.01) groups.push([say(t("It spent {money} at API prices.", { money: moneyWords(recap.cost) }))]);
    if (recap.asking) groups.push([say(t("A question is open."))]);
    if (recap.said) groups.push([say(t("Its last answer:")), quoted(quote(recap.said))]);
  }
  groups.push([say(t("That's all for now."))]);
  return groups.map((sentences, i) => ({ key: `line-${i}`, title, sentences }));
}
