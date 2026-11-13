// The prompts list: a conversation read prompt by prompt, newest first — when it was sent, the messages sent while the
// agent worked on it, how long the agent took, the tokens it wrote, the files it changed, the commands it ran, the gist
// of its reply. After Over9k's Timeline. Pure, over the turns the chat already holds; PromptsList draws it.

import { tokFine } from "@/features/usage/format";
import { t, tn } from "@/lib/i18n";
import { spent } from "@/lib/time";
import type { Turn } from "./api";
import { COMPACTION_PREFIX } from "./timeline";
import { EDIT_TOOLS, toolPath } from "./footprint";
import { plain } from "./said";

export type Sent = { at?: string; text: string; from?: string };

export type PromptCard = {
  n: number; // its number, the oldest prompt 1
  id: string; // the prompt's first record: where opening the card jumps the log
  at?: string; // when it was sent
  text: string; // the prompt as typed
  also: Sent[]; // the messages sent while the agent worked on it
  done?: number; // unix ms of the answer's last record; none while nothing answered
  took: number; // seconds from the prompt to that record
  output: number; // the output tokens the agent wrote on it
  edits: string[]; // the files it changed, in the order first touched
  commands: number; // the shell commands it ran
  reply: string; // what the agent said last on it: the text after its last tool call, else its last text
};

export const GIST_MAX = 140;
const SHORT_SENTENCE = 25;
const SHELL_TOOLS = new Set(["Bash", "exec_command", "shell", "local_shell"]);
const NOT_A_PROMPT = [COMPACTION_PREFIX, "[Request interrupted"];

const textOf = (turn: Turn) =>
  turn.parts
    .filter((p) => p.type === "text")
    .map((p) => p.text ?? "")
    .join("\n\n")
    .trim();

// promptCards reads the turns as one card per prompt of the person's, oldest first; an interruption's note and a
// compaction's summary are no prompts, and what follows them stays with the card before
export function promptCards(turns: Turn[]): PromptCard[] {
  const cards: PromptCard[] = [];
  let card: PromptCard | null = null;
  let asked = 0; // the last user turn's time, which an answer's took counts from
  let texts: string[] = [];
  let lastText = "";
  const settle = () => {
    if (card) card.reply = texts.join("\n\n") || lastText;
  };
  for (const turn of turns) {
    const sent = turn.parts.filter((p) => p.type === "queued").map((p) => ({ at: p.at, text: p.text ?? "", from: p.from }));
    if (turn.role === "user") {
      asked = turn.ts ? Date.parse(turn.ts) || 0 : 0;
      const text = textOf(turn);
      const prompt = text !== "" && !NOT_A_PROMPT.some((prefix) => text.startsWith(prefix));
      if (prompt || (!card && sent.length > 0)) {
        settle();
        card = { n: cards.length + 1, id: turn.ids?.[0] ?? "", at: turn.ts, text: prompt ? text : sent.shift()!.text, also: [], took: 0, output: 0, edits: [], commands: 0, reply: "" };
        cards.push(card);
        texts = [];
        lastText = "";
      }
      if (card) card.also.push(...sent);
      continue;
    }
    if (!card) continue;
    card.also.push(...sent);
    card.output += turn.output ?? 0;
    if (turn.took && asked) {
      const done = asked + turn.took * 1000;
      if (!card.done || done > card.done) card.done = done;
    }
    for (const part of turn.parts) {
      if (part.type === "tool") {
        texts = [];
        const name = part.tool_name ?? "";
        if (SHELL_TOOLS.has(name)) card.commands++;
        const path = EDIT_TOOLS.has(name) ? toolPath(part) : null;
        if (path && !card.edits.includes(path)) card.edits.push(path);
      } else if (part.type === "text" && part.text?.trim()) {
        texts.push(part.text.trim());
        lastText = part.text.trim();
      }
    }
  }
  settle();
  for (const c of cards) {
    const at = c.at ? Date.parse(c.at) || 0 : 0;
    c.took = c.done && at ? Math.max(0, (c.done - at) / 1000) : 0;
  }
  return cards;
}

// plainText is Markdown as one line of words: fences dropped, every line's marks stripped
export function plainText(markdown: string): string {
  return markdown
    .replace(/```[\s\S]*?(```|$)/g, " ")
    .split("\n")
    .map(plain)
    .filter(Boolean)
    .join(" ")
    .trim();
}

// gist is the start of a reply: its first sentence, two when the first is a word or two ("Done."), cut at GIST_MAX
export function gist(reply: string): string {
  const sentences = plainText(reply).split(/(?<=[.!?])\s+/);
  const first = sentences[0] ?? "";
  const shown = first.length < SHORT_SENTENCE && sentences[1] ? `${first} ${sentences[1]}` : first;
  return shown.length > GIST_MAX ? `${shown.slice(0, GIST_MAX - 1).trimEnd()}…` : shown;
}

// alsoWords is the line under a prompt for the messages sent while the agent worked: "+ 2 more messages: … · …"
export function alsoWords(also: Sent[]): string {
  if (also.length === 0) return "";
  const text = also.map((m) => plainText(m.text)).join(" · ");
  return tn(also.length, "+ {n} more message: {text}", "+ {n} more messages: {text}", { text });
}

export type Live = { seconds: number | null; tokens?: number }; // the turn under way, by the spinner

// facts is what a card says it came to, every zero left out: "done 16:02 · 4 min · ↓ 9.1k tokens · 3 files · 6 commands";
// the turn under way leads with "working" (lead) and its time and tokens so far
export function facts(card: PromptCard, live: Live | null, clock: (ms: number) => string): { lead: string; rest: string[] } {
  const tokens = live?.tokens && live.tokens > card.output ? live.tokens : card.output;
  const rest = [
    live ? (live.seconds !== null ? spent(live.seconds) : "") : card.took ? spent(card.took) : "",
    tokens ? t("↓ {n} tokens", { n: tokFine(tokens) }) : "",
    card.edits.length ? tn(card.edits.length, "{n} file", "{n} files") : "",
    card.commands ? tn(card.commands, "{n} command run", "{n} commands run") : "",
  ].filter(Boolean);
  if (live) return { lead: t("working"), rest };
  return { lead: "", rest: card.done ? [t("done {time}", { time: clock(card.done) }), ...rest] : rest };
}

// totals is the list's head: the output tokens of every prompt, the live one's by the spinner when it says more
export function totalOutput(cards: PromptCard[], live: Live | null): number {
  return cards.reduce((sum, card, i) => sum + (live && i === cards.length - 1 && (live.tokens ?? 0) > card.output ? live.tokens! : card.output), 0);
}

export type Walk = { at: number; open: number | null };

// walk is the list's keys over the cards, newest first: ↑ ↓ move the pick, an open card moving with it; Enter opens or
// shuts the picked one; Esc shuts the open one. null for a key it leaves alone.
export function walk(state: Walk, key: string, count: number): Walk | null {
  if (count === 0) return null;
  const at = Math.min(Math.max(state.at, 0), count - 1);
  switch (key) {
    case "ArrowDown":
    case "ArrowUp": {
      const next = Math.min(count - 1, Math.max(0, at + (key === "ArrowDown" ? 1 : -1)));
      return { at: next, open: state.open !== null ? next : null };
    }
    case "Enter":
      return { at, open: state.open === at ? null : at };
    case "Escape":
      return state.open !== null ? { at, open: null } : null;
  }
  return null;
}
