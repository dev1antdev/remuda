// What the agents ask the person, read off their panes' screens (`GET /api/question`): the dialog's head and the first
// line of its detail, which the sidebar's "Needs you" quotes; how often and for how many panes the readings go. Pure;
// useQuestions.ts is the hook it reads through.

import type { Question } from "@/features/chat/api";
import { questionLines } from "@/features/chat/question";

// QUESTIONS_EVERY is the pace of a round of readings; MAX_ASKING how many panes one round reads, the first ones given
export const QUESTIONS_EVERY = 10_000;
export const MAX_ASKING = 8;

// Asker is a pane asking something: the key its answer is kept by, its machine and pane
export type Asker = { key: string; machine: string; pane: string };

// ASK_MAX is how much of a question its line holds
const ASK_MAX = 90;

// questionLine is what an asking agent asks, in a line: the dialog's head and the first line of its detail ("Bash
// command · rm -rf build"), else the line that asks
export function questionLine(question: Question | null | undefined): string {
  if (!question?.text) return "";
  const { head, detail, ask } = questionLines(question.text);
  const first = detail.split("\n")[0] ?? "";
  const text = head ? (first ? `${head} · ${first}` : head) : ask;
  return cutAt(text, ASK_MAX);
}

// cutAt shortens a text to max characters at a word, never between the halves of a surrogate pair, with an ellipsis
export function cutAt(raw: string, max: number): string {
  const text = raw.trim();
  if (text.length <= max) return text;
  const space = text.slice(0, max).lastIndexOf(" ");
  let end = space > max / 2 ? space : max;
  const code = text.charCodeAt(end - 1);
  if (code >= 0xd800 && code <= 0xdbff) end -= 1;
  return `${text.slice(0, end).trimEnd()}…`;
}

// sameQuestions says whether two readings of what the asking agents hold up say the same, so an unchanged one draws nothing
export function sameQuestions(a: Map<string, string>, b: Map<string, string>): boolean {
  if (a.size !== b.size) return false;
  for (const [key, text] of a) if (b.get(key) !== text) return false;
  return true;
}

// askedOf is the panes a round reads, the first MAX_ASKING, and the signature that says when they changed
export function askedOf(askers: Asker[]): { wanted: Asker[]; signature: string } {
  const wanted = askers.slice(0, MAX_ASKING);
  return { wanted, signature: wanted.map((a) => a.key).join(" ") };
}
