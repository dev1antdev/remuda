// The shape of a conversation in time: one segment per prompt and its answer, the moments worth a look — the first
// failed call, the longest call, a loop of identical calls, the questions asked, a compaction — and how long the
// person was away. Pure, over the turns the chat already holds; TimelineBar draws it.

import { spent } from "@/lib/time";
import type { Part, Turn } from "./api";
import { toolLabel } from "./footprint";
import { t } from "@/lib/i18n";

export const SQUEEZE_MS = 10 * 60 * 1000; // a gap longer than this is drawn this long: one lunch break must not flatten the day
export const LONG_WAIT_MS = 30 * 60 * 1000; // a gap from an answer to the next prompt that counts as the person away
export const LOOP_FROM = 3; // identical consecutive calls that make a loop
export const COMPACTION_PREFIX = "This session is being continued from a previous conversation";

export type Segment = {
  id: string; // the prompt turn's first record: where a click lands
  start: number; // ms, the prompt
  end: number; // ms, the end of the answer's last record
  prompt: string; // the prompt's first line
  took: number; // seconds, the answer's
  x: number; // 0..1 along the squeezed bar
  w: number; // 0..1
};

export type HighlightKind = "error" | "longest" | "loop" | "question" | "compaction";

export type Highlight = {
  kind: HighlightKind;
  id: string; // the turn's first record: where a click lands
  at: number; // ms
  x: number; // 0..1 along the squeezed bar
  label: string;
  detail: string;
  count: number; // the loop's calls, the failed calls behind the first error, 1 otherwise
};

export type Timeline = {
  segments: Segment[];
  highlights: Highlight[]; // in time order
  waits: number; // gaps over LONG_WAIT_MS between an answer and the next prompt
  total: number; // seconds from the first prompt to the end of the last answer
};

export type Chip = { kind: HighlightKind; id: string; text: string; title: string };

type Mark = Omit<Highlight, "x">;
type Run = { key: string; count: number; id: string; at: number; label: string };

const when = (ts?: string): number => (ts ? Date.parse(ts) || 0 : 0);

const firstText = (turn: Turn): string => turn.parts.find((p) => p.type === "text")?.text ?? "";

const firstLine = (text: string): string => {
  const line = text.split("\n").find((l) => l.trim()) ?? "";
  return line.length > 80 ? line.slice(0, 79).trimEnd() + "…" : line.trim();
};

const callWords = (part: Part): string => {
  const { name, hint } = toolLabel(part);
  return hint ? `${name} ${hint}` : name;
};

const callKey = (part: Part): string => `${part.tool_name ?? ""}\n${typeof part.input === "string" ? part.input : JSON.stringify(part.input ?? null)}`;

// timeline lays the turns out in time and finds the moments worth a look
export function timeline(turns: Turn[]): Timeline {
  const segments: Omit<Segment, "x" | "w">[] = [];
  const marks: Mark[] = [];
  let failed = 0;
  let firstError: Mark | null = null;
  let longest: (Mark & { took: number }) | null = null;
  let run: Run | null = null;
  const closeRun = () => {
    if (run && run.count >= LOOP_FROM) marks.push({ kind: "loop", id: run.id, at: run.at, label: `Loop ×${run.count}`, detail: `${run.label}: ${run.count} identical calls in a row`, count: run.count });
    run = null;
  };
  for (let i = 0; i < turns.length; i++) {
    const turn = turns[i]!;
    const id = turn.ids?.[0] ?? "";
    const ts = when(turn.ts);
    if (turn.role === "user") {
      closeRun();
      const text = firstText(turn);
      if (text.startsWith(COMPACTION_PREFIX)) marks.push({ kind: "compaction", id, at: ts, label: t("Compacted"), detail: t("the conversation went on in a new file, from a summary of the one before"), count: 1 });
      const answer = turns[i + 1];
      if (answer?.role === "assistant" && ts) {
        const end = answer.took ? ts + answer.took * 1000 : when(answer.ts) || ts;
        segments.push({ id, start: ts, end: Math.max(end, ts), prompt: firstLine(text), took: answer.took ?? 0 });
      }
      continue;
    }
    for (const part of turn.parts) {
      if (part.type !== "tool") continue;
      const at = when(part.at) || ts;
      if (part.is_error) {
        failed++;
        if (!firstError) firstError = { kind: "error", id, at, label: t("First error"), detail: callWords(part), count: 1 };
      }
      if (part.took && (!longest || part.took > longest.took)) longest = { kind: "longest", id, at, label: `Longest call ${spent(part.took)}`, detail: callWords(part), count: 1, took: part.took };
      if (part.tool_name === "AskUserQuestion") marks.push({ kind: "question", id, at, label: t("Question"), detail: callWords(part), count: 1 });
      const key = callKey(part);
      if (run && run.key === key) run.count++;
      else {
        closeRun();
        run = { key, count: 1, id, at, label: callWords(part) };
      }
    }
  }
  closeRun();
  if (firstError) {
    firstError.count = failed;
    if (failed > 1) firstError.detail = `${firstError.detail} · ${failed} failed calls in all`;
    marks.push(firstError);
  }
  if (longest) marks.push({ kind: "longest", id: longest.id, at: longest.at, label: longest.label, detail: longest.detail, count: 1 });
  marks.sort((a, b) => a.at - b.at);

  let waits = 0;
  for (let i = 1; i < segments.length; i++) if (segments[i]!.start - segments[i - 1]!.end > LONG_WAIT_MS) waits++;

  const place = squeezer(segments.flatMap((s) => [s.start, s.end]));
  const first = segments[0]?.start ?? 0;
  const last = segments.reduce((m, s) => Math.max(m, s.end), first);
  return {
    segments: segments.map((s) => ({ ...s, x: place(s.start), w: place(s.end) - place(s.start) })),
    highlights: marks.map((m) => ({ ...m, x: place(m.at) })),
    waits,
    total: (last - first) / 1000,
  };
}

// squeezer maps a moment to 0..1 along a bar where every gap between the given points over SQUEEZE_MS is drawn as
// SQUEEZE_MS: time inside a span stays proportional, a break shrinks to a notch
export function squeezer(points: number[]): (t: number) => number {
  const sorted = [...new Set(points.filter((p) => p > 0))].sort((a, b) => a - b);
  if (sorted.length === 0) return () => 0;
  const pos: number[] = [0];
  for (let i = 1; i < sorted.length; i++) pos.push(pos[i - 1]! + Math.min(sorted[i]! - sorted[i - 1]!, SQUEEZE_MS));
  const span = Math.max(pos[pos.length - 1]!, 1);
  return (t: number) => {
    if (t <= sorted[0]!) return 0;
    if (t >= sorted[sorted.length - 1]!) return pos[pos.length - 1]! / span;
    let i = 1;
    while (sorted[i]! < t) i++;
    const a = sorted[i - 1]!;
    const b = sorted[i]!;
    return (pos[i - 1]! + ((t - a) / (b - a)) * (pos[i]! - pos[i - 1]!)) / span;
  };
}

// chips is the row under the bar: one word per kind of moment, the first error, the longest call, each loop (three at most),
// the questions as one, the compactions as one — each a jump to its turn
export function chips(line: Timeline): Chip[] {
  const out: Chip[] = [];
  const error = line.highlights.find((h) => h.kind === "error");
  if (error) out.push({ kind: "error", id: error.id, text: error.count > 1 ? `First of ${error.count} errors` : "First error", title: error.detail });
  const longest = line.highlights.find((h) => h.kind === "longest");
  if (longest) out.push({ kind: "longest", id: longest.id, text: longest.label, title: longest.detail });
  for (const loop of line.highlights.filter((h) => h.kind === "loop").slice(0, 3)) out.push({ kind: "loop", id: loop.id, text: loop.label, title: loop.detail });
  const questions = line.highlights.filter((h) => h.kind === "question");
  if (questions.length === 1) out.push({ kind: "question", id: questions[0]!.id, text: t("1 question"), title: t("the agent asked once; the mark is where") });
  if (questions.length > 1) out.push({ kind: "question", id: questions[0]!.id, text: `${questions.length} questions`, title: `the agent asked ${questions.length} times; this goes to the first` });
  const compactions = line.highlights.filter((h) => h.kind === "compaction");
  if (compactions.length) out.push({ kind: "compaction", id: compactions[0]!.id, text: compactions.length === 1 ? "Compacted" : `Compacted ×${compactions.length}`, title: compactions[0]!.detail });
  return out;
}

// summary is the bar's own title: the session's length and the breaks in it
export function summary(line: Timeline): string {
  const breaks = line.waits ? `, ${line.waits} break${line.waits === 1 ? "" : "s"} over 30 min` : "";
  return `${spent(line.total)} from the first prompt to the end of the last answer${breaks}; a gap over 10 min is drawn as 10`;
}
