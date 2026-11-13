// The agent's last half hour as a ribbon: stretches of one colour each for what it read, wrote, ran in a shell, waited on,
// thought, or nothing at all, now at the right — from the times its conversation's records carry, a call's own and a
// text's or thought's. After Orchi's Herd panel. Pure; Ribbon draws it.

import { took as tookWords, spent, ago } from "@/lib/time";
import { t, tc, tn } from "@/lib/i18n";
import type { Part, Turn } from "./api";
import { EDIT_TOOLS, isQuestion, shownPath, toolLabel, toolPath } from "./footprint";
import { SPAWN_TOOLS } from "./subagents";
import { COMPACTION_PREFIX } from "./timeline";

export const RIBBON_MS = 30 * 60 * 1000;
export const QUIET_MS = 3 * 60 * 1000; // a turn with no call running and nothing written this long is quiet
export const HANDOFF_MS = 10 * 1000; // an Agent call open this long is the lead waiting on its subagents
export const HIT_PX = 4; // how near a press must land to a narrow call to name it

// one colour, one meaning: the base ones fill the gaps between the calls, the calls lie on top
export type Tone = "think" | "read" | "write" | "shell" | "wait" | "tool" | "quiet" | "unread";

export type Stretch = {
  key: string; // stable across redraws: what the press holds on to
  tone: Tone;
  from: number; // ms
  to: number; // ms
  call?: { name: string; hint: string; file: string | null; running: boolean; failed: boolean };
  turn: string; // the first record of its prompt (else its answer): where the lens jumps
};

// TurnFigures are what the latest turn came to: how long, how many calls, how many failed, and whether it still goes
export type TurnFigures = { took: number; tools: number; failed: number; live: boolean };

export type RibbonModel = { from: number; now: number; stretches: Stretch[]; turn: TurnFigures | null };

// Live is how the agent stands now: at work, asking the person, or at rest
export type Live = "run" | "input" | null;

const READ_TOOLS = new Set(["Read", "Grep", "Glob", "LS", "WebFetch", "WebSearch", "NotebookRead", "read_file", "read_many_files", "list_directory", "glob", "search_file_content", "grep", "web_fetch", "google_web_search", "web_search", "view_image"]);
const WRITE_TOOLS = new Set([...EDIT_TOOLS, "apply_patch", "write_file", "replace", "edit"]);
const SHELL_TOOLS = new Set(["Bash", "BashOutput", "KillShell", "KillBash", "exec_command", "shell", "local_shell", "write_stdin", "run_shell_command"]);
// the words a call is answered with when it waited on the person: refused for want of anyone to approve it, or refused by them
const WAITED_OPENINGS = ["Permission for this tool use was denied", "The user doesn't want to proceed with this tool use"];

const when = (ts?: string): number => (ts ? Date.parse(ts) || 0 : 0);

function waited(part: Part): boolean {
  if (!part.is_error || typeof part.output !== "string") return false;
  const out = part.output.trim().replace(/^Error: /, "");
  return WAITED_OPENINGS.some((opening) => out.startsWith(opening)) || (out.startsWith("Permission to use ") && out.includes("don't ask mode"));
}

// toneOf is the colour of a call: what it asked, a refusal that waited on the person, a subagent run, else its kind
export function toneOf(part: Part, length: number | null): Tone {
  const name = part.tool_name ?? "";
  if (isQuestion(name) || waited(part)) return "wait";
  if (SPAWN_TOOLS.has(name)) return length === null || length >= HANDOFF_MS ? "wait" : "tool";
  if (READ_TOOLS.has(name)) return "read";
  if (WRITE_TOOLS.has(name)) return "write";
  if (SHELL_TOOLS.has(name)) return "shell";
  return "tool";
}

// Exchange is a prompt and its answer in time, or an answer whose prompt lies before the turns read
type Exchange = { start: number; parts: Part[]; took?: number; answered: number; turn: string };

function exchangesOf(turns: Turn[]): Exchange[] {
  const out: Exchange[] = [];
  for (let i = 0; i < turns.length; i++) {
    const turn = turns[i]!;
    if (turn.role === "user") {
      const start = when(turn.ts);
      const answer = turns[i + 1]?.role === "assistant" ? turns[i + 1] : undefined;
      if (answer) i++;
      if (start) out.push({ start, parts: answer?.parts ?? [], took: answer?.took, answered: when(answer?.ts), turn: turn.ids?.[0] ?? "" });
      continue;
    }
    const start = Math.min(...[when(turn.ts), ...turn.parts.map((p) => when(p.at))].filter((x) => x > 0));
    if (Number.isFinite(start)) out.push({ start, parts: turn.parts, answered: start, turn: turn.ids?.[0] ?? "" });
  }
  return out;
}

type Call = { part: Part; from: number; to: number; running: boolean };

// stretchesOf lays one exchange out: its calls, and the gaps between them as thought — quiet past QUIET_MS without a sign
// of life — with the time after the last record waiting on the person while the agent asks
function stretchesOf(x: Exchange, live: Live, last: boolean, now: number): { calls: Call[]; base: Stretch[]; end: number } {
  const going = last && live !== null;
  const signs: number[] = [x.start];
  const calls: Call[] = [];
  for (const part of x.parts) {
    const at = when(part.at);
    if (at) signs.push(at);
  }
  const lastSign = Math.max(...signs, x.answered);
  let end = going ? now : x.took ? x.start + x.took * 1000 : lastSign;
  for (const part of x.parts) {
    if (part.type !== "tool") continue;
    const from = Math.max(when(part.at) || x.start, x.start);
    const done = part.output !== undefined;
    const to = done ? from + (part.took ?? 0) * 1000 : going ? now : Math.max(end, from);
    calls.push({ part, from, to, running: !done && going });
    signs.push(to);
    if (!going) end = Math.max(end, to);
  }
  const sorted = [...new Set(signs.filter((s) => s >= x.start && s <= end))].sort((a, b) => a - b);
  const busy = [...calls].sort((a, b) => a.from - b.from);
  const base: Stretch[] = [];
  const asking = going && live === "input" && !calls.some((c) => c.running);
  const add = (tone: Tone, from: number, to: number) => {
    if (to <= from) return;
    const prev = base[base.length - 1];
    if (prev && prev.tone === tone && prev.to === from) prev.to = to;
    else base.push({ key: `${tone}:${from}`, tone, from, to, turn: x.turn });
  };
  const gap = (from: number, to: number) => {
    const points = [from, ...sorted.filter((s) => s > from && s < to), to];
    for (let i = 1; i < points.length; i++) {
      const a = points[i - 1]!;
      const b = points[i]!;
      if (asking && b === end && i === points.length - 1) add("wait", a, b);
      else if (b - a <= QUIET_MS) add("think", a, b);
      else {
        add("think", a, a + QUIET_MS);
        add("quiet", a + QUIET_MS, b);
      }
    }
  };
  let cursor = x.start;
  for (const call of busy) {
    if (call.from > cursor) gap(cursor, call.from);
    cursor = Math.max(cursor, call.to);
  }
  if (end > cursor) gap(cursor, end);
  return { calls, base, end };
}

// ribbon is the last RIBBON_MS of a conversation: cut says the turns read may start after the window does (a tile's tail,
// a file a compaction began), so the time before the first of them is not read rather than idle
export function ribbon(turns: Turn[], { now, live, cut }: { now: number; live: Live; cut: boolean }): RibbonModel {
  const from = now - RIBBON_MS;
  const exchanges = exchangesOf(turns);
  const stretches: Stretch[] = [];
  const calls: Stretch[] = [];
  let figures: TurnFigures | null = null;
  exchanges.forEach((x, i) => {
    const last = i === exchanges.length - 1;
    // a long conversation in the lens: the exchanges long over before the window are not laid out at all
    if (!last && exchanges[i + 1]!.start < from && exchanges[i + 1]!.start >= x.start + (x.took ?? 0) * 1000) return;
    const laid = stretchesOf(x, live, last, now);
    if (last) {
      const tools = laid.calls.length;
      figures = { took: Math.max(0, (laid.end - x.start) / 1000), tools, failed: laid.calls.filter((c) => c.part.is_error).length, live: live !== null };
    }
    if (laid.end < from) return;
    stretches.push(...laid.base);
    for (const c of laid.calls) {
      if (c.to < from) continue;
      const label = toolLabel(c.part);
      const length = c.running ? null : c.to - c.from;
      const tone = c.running && live === "input" ? "wait" : toneOf(c.part, length);
      calls.push({ key: `${c.part.tool_id || c.part.tool_name}:${c.from}`, tone, from: c.from, to: c.to, turn: x.turn, call: { name: label.name, hint: label.hint, file: toolPath(c.part), running: c.running, failed: !!c.part.is_error } });
    }
  });
  const first = exchanges[0];
  const startsLate = !!first && (cut || (turns[0]?.role === "user" && (turns[0].parts.find((p) => p.type === "text")?.text ?? "").startsWith(COMPACTION_PREFIX)));
  if (startsLate && first.start > from) stretches.unshift({ key: "unread", tone: "unread", from, to: first.start, turn: "" });
  const clip = (s: Stretch): Stretch => ({ ...s, from: Math.max(s.from, from), to: Math.min(s.to, now) });
  const shown = [...stretches.filter((s) => s.to > from && s.to - s.from > 0).map(clip), ...calls.map(clip)];
  return { from, now, stretches: shown.filter((s) => s.to >= s.from), turn: figures };
}

// stretchAt is the stretch under a point a share `at` along the ribbon, `width` px wide: a call over the gap it lies in,
// a narrow call found within HIT_PX, the nearest of several
export function stretchAt(model: RibbonModel, at: number, width: number): Stretch | null {
  const span = model.now - model.from;
  if (span <= 0 || width <= 0) return null;
  const time = model.from + at * span;
  const slack = (HIT_PX / width) * span;
  let best: { s: Stretch; off: number } | null = null;
  for (const s of model.stretches) {
    const near = s.call ? slack : 0;
    const off = time < s.from - near ? Infinity : time > s.to + near ? Infinity : time < s.from ? s.from - time : time > s.to ? time - s.to : 0;
    if (!Number.isFinite(off)) continue;
    const better = !best || (s.call && !best.s.call) || (!!s.call === !!best.s.call && (off < best.off || (off === best.off && s.from > best.s.from)));
    if (better) best = { s, off };
  }
  return best?.s ?? null;
}

// ordered is the stretches as the keyboard walks them: by start, a call after the gap it lies in
export function ordered(model: RibbonModel): Stretch[] {
  return [...model.stretches].sort((a, b) => a.from - b.from || (a.call ? 1 : 0) - (b.call ? 1 : 0));
}

// stepFrom is the stretch before or after the one held, the first or last when none is
export function stepFrom(model: RibbonModel, key: string | null, by: 1 | -1): Stretch | null {
  const list = ordered(model);
  if (list.length === 0) return null;
  const i = key === null ? -1 : list.findIndex((s) => s.key === key);
  if (i < 0) return by > 0 ? list[0]! : list[list.length - 1]!;
  return list[Math.min(list.length - 1, Math.max(0, i + by))]!;
}

// toneWords names each colour, in the line under the ribbon beside its icon, so the colour is never the only cue
export const toneWords = (tone: Tone): string =>
  ({
    think: tc("ribbon", "Thinking"),
    read: tc("ribbon", "Reading"),
    write: tc("ribbon", "Writing"),
    shell: tc("ribbon", "Shell"),
    wait: tc("ribbon", "Waiting"),
    tool: tc("ribbon", "Other tools"),
    quiet: tc("ribbon", "Quiet"),
    unread: tc("ribbon", "Not read"),
  })[tone];

const cutLeft = (text: string, n: number) => (text.length > n ? `…${text.slice(-(n - 1))}` : text);

// stretchWords say what a stretch was: "Edit · src/app.go · 12 s · 3 min ago", "Quiet · 4 min · nothing written"
export function stretchWords(s: Stretch, now: number, dir: string, unread: string): string {
  const length = (s.to - s.from) / 1000;
  const since = ago(s.to, now);
  if (s.call) {
    const what = s.call.file ? cutLeft(shownPath(s.call.file, dir), 60) : s.call.hint;
    const lasted = s.call.running ? t("{time} so far", { time: tookWords(length) }) : tookWords(length);
    return [s.call.name, what, lasted, s.call.failed ? t("failed") : "", s.call.running ? "" : since].filter(Boolean).join(" · ");
  }
  if (s.tone === "unread") return unread;
  if (s.tone === "quiet") return t("{time} with no call running and nothing written · {ago}", { time: spent(length), ago: since });
  if (s.tone === "wait") return t("{time} waiting for you", { time: spent(length) });
  return `${spent(length)} · ${since}`;
}

// turnWords is the line under the ribbon when nothing is pointed at: "1 min 7 s, 18 tools, 1 failed"
export function turnWords(turn: TurnFigures | null): string {
  if (!turn) return t("Nothing in the last 30 minutes");
  const tools = turn.tools ? tn(turn.tools, "{n} tool", "{n} tools") : t("no tools");
  const failed = turn.failed ? `, ${t("{n} failed", { n: turn.failed })}` : "";
  return turn.live ? t("{time} so far, {tools}{failed}", { time: spent(turn.took), tools, failed }) : `${spent(turn.took)}, ${tools}${failed}`;
}
