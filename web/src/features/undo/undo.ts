// The words and choices of undoing a turn, pure: which turn is offered first, what the undo does to each file, what it
// said it did, and which answer of a conversation a kept turn belongs to.

import type { Turn } from "@/features/chat/api";
import { t } from "@/lib/i18n";
import { plural } from "@/lib/text";
import { spent, whenShort } from "@/lib/time";
import { VENDOR_NAME } from "@/ui/brand";
import type { ListedTurn, UndoFile, UndoPlan, UndoResult } from "./api";

// undoable says a turn can still be undone as far as the cockpit knows; the folder itself is read before anything goes
export const undoable = (turn: ListedTurn): boolean => !turn.undone && !turn.why;

// defaultTurn is the newest turn that can still be undone, else the newest
export function defaultTurn(turns: ListedTurn[]): ListedTurn | undefined {
  return turns.find(undoable) ?? turns[0];
}

// turnLabel names a turn in the picker: its agent and when it ended
export function turnLabel(turn: ListedTurn): string {
  const agent = turn.agent ? (VENDOR_NAME[turn.agent] ?? turn.agent) : t("An agent");
  return `${agent} · ${whenShort(turn.ended)}`;
}

// turnHint is a turn's size beside its name, or why it is out of reach
export function turnHint(turn: ListedTurn): string {
  if (turn.undone) return t("undone");
  if (turn.why) return t("cannot be undone");
  const files = turn.files;
  const commits = turn.commits?.length ?? 0;
  const seconds = Math.max(0, (Date.parse(turn.ended) - Date.parse(turn.started)) / 1000);
  return [files ? plural(files, "file") : "", commits ? plural(commits, "commit") : "", seconds >= 1 ? spent(seconds) : ""].filter(Boolean).join(" · ");
}

// fileWords is what the undo does to a file, with the commits reverted or left as they are
export function fileWords(file: UndoFile, revert: boolean): string {
  if (file.committed && !revert) {
    if (file.dirty_at) return t("left as it is: only reverting the commits puts it back");
    if (!file.dirty_to) return t("stays as the commits have it");
    return t("back to the last commit");
  }
  switch (file.was) {
    case "added":
      return t("removed: the turn made it");
    case "deleted":
      return t("brought back");
    case "mode":
      return t("its mode put back");
    default:
      return t("put back");
  }
}

// canRevert says the turn made commits and they can be reverted as the folder stands
export const canRevert = (plan: UndoPlan | null): boolean => !!plan && !plan.refused && (plan.turn.commits?.length ?? 0) > 0 && !plan.revert_why;

// nothingToDo says the undo would change nothing: only commits made, and their revert left out
export const nothingToDo = (plan: UndoPlan, revert: boolean): boolean => !(revert && canRevert(plan)) && plan.uncommitted === 0;

// undoneWords is the toast after an undo
export function undoneWords(result: UndoResult): string {
  const files = plural(result.files, "file");
  const reverted = result.reverted?.length ?? 0;
  if (result.files > 0 && reverted > 0) return t("Undone: {files} put back, {commits} reverted", { files, commits: plural(reverted, "commit") });
  if (reverted > 0) return t("Undone: {commits} reverted", { commits: plural(reverted, "commit") });
  return t("Undone: {files} put back", { files });
}

// pushedWords warns that the commits about to be reverted are on a remote already
export function pushedWords(pushed: string[]): string {
  return t("Already pushed to {branches}: the reverts are new commits, to be pushed like any other.", { branches: pushed.join(", ") });
}

// sentence is a reason the server gave, as a sentence on its own: its first letter raised
export const sentence = (text: string): string => (text ? text[0]!.toUpperCase() + text.slice(1) : text);

// SLACK_MS is how far apart a kept turn's ends and an answer's may sit and still be the same turn: herdr says working as
// the prompt lands and rest a moment after the last record
export const SLACK_MS = 30_000;

type Span = { key: string; to: number };

// answerSpans is when each answer of a conversation ended: its prompt's moment plus the seconds it took, else its own
function answerSpans(turns: Turn[]): Span[] {
  const spans: Span[] = [];
  let prompt = NaN;
  for (const turn of turns) {
    const at = turn.ts ? Date.parse(turn.ts) : NaN;
    if (turn.role === "user") {
      prompt = at;
      continue;
    }
    const key = turn.ids?.[0];
    if (!key || Number.isNaN(at)) continue;
    spans.push({ key, to: turn.took && !Number.isNaN(prompt) ? prompt + turn.took * 1000 : at });
  }
  return spans;
}

// matchTurns pairs the pane's kept turns with the answers of its conversation: each kept turn goes to the last answer
// that ended within it, by the answer's first record; an answer takes one kept turn, the newest
export function matchTurns(turns: Turn[], kept: ListedTurn[], paneId: string): Map<string, ListedTurn> {
  const spans = answerSpans(turns);
  const matched = new Map<string, ListedTurn>();
  for (const turn of kept) {
    if (turn.pane !== paneId) continue;
    const from = Date.parse(turn.started) - SLACK_MS;
    const to = Date.parse(turn.ended) + SLACK_MS;
    let last: Span | undefined;
    for (const span of spans) if (span.to >= from && span.to <= to && !matched.has(span.key)) last = span;
    if (last) matched.set(last.key, turn);
  }
  return matched;
}
