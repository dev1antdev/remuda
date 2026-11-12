// Models, and modes of a model, being retired. The server's table (internal/retire) rides on the roster as
// `retirements`, and each machine's `retiring` says what starts or runs there on one; the ⓘ turns those into one
// concern per retirement. A form that edits a model asks retiredFor by the server's own rule: the model is a --model
// in the extra arguments, else the field, else the machine's default; fast mode is what the arguments' --settings
// say, else the machine's saved choice. Pure, tested.

import { t, tn } from "@/lib/i18n";
import { fmtDate } from "@/lib/time";

export type Retirement = { agent: string; model: string; name: string; mode?: string; on: string; replacement: string; replacement_name: string; source: string };

// RetiringHit is what is set to a retirement on a machine: a recipe, an agent schedule, a project's agents, one of
// its stages, or a pane whose meter read it; no names, the forms say their own. The retirement goes by its key, its
// agent, model and mode, which the roster's table answers.
export type RetiringHit = { agent: string; model: string; mode?: string; kind: "recipe" | "schedule" | "project" | "stage" | "pane"; id: string; machine: string; failing: boolean };

// retirementOf is the table's entry a hit names, undefined for one the table no longer holds
const retirementOf = (table: Retirement[], hit: RetiringHit): Retirement | undefined =>
  table.find((r) => r.agent === hit.agent && r.model === hit.model && (r.mode ?? "") === (hit.mode ?? ""));

// Defaults is Claude Code's choice on a machine for a session that names none, as its vitals read the user settings
export type Defaults = { model?: string; fast?: boolean };

// ModelSetting is what a form holds: the agent, its model field, the extra arguments, the machine ("" for any)
export type ModelSetting = { agent: string; model: string; args: string; machine: string };

// Found is the retirement a setting is on the way of, why, and the machines where it would be ("" for wherever)
export type Found = { retirement: Retirement; model: "field" | "arguments" | "default"; fast: "arguments" | "default" | ""; machines: string[] };

// canonical is a model id as the table names it: lowercase, past a provider's prefix, without "[1m]", "@date",
// Bedrock's "-v1:0" or a date; an alias ("opus") stays as it is, naming the newest
export function canonical(model: string): string {
  let m = model.trim().toLowerCase();
  const at = m.indexOf("claude-");
  if (at > 0) m = m.slice(at);
  m = m.replace(/\[[^\]]*\]$/, "");
  const vertex = m.indexOf("@");
  if (vertex > 0) m = m.slice(0, vertex);
  return m.replace(/-v\d+(:\d+)?$/, "").replace(/-\d{8}$/, "");
}

// unescaped is the arguments with a backslash before a double quote dropped, as a shell drops it inside double quotes
const unescaped = (args: string): string => args.replaceAll('\\"', '"');

// fastIn is what the extra arguments say of fast mode through a --settings JSON, the last word winning; undefined for nothing
export function fastIn(args: string): boolean | undefined {
  const said = [...unescaped(args).matchAll(/"fastMode"\s*:\s*(true|false)/g)];
  return said.length ? said[said.length - 1]![1] === "true" : undefined;
}

// modelIn is a model the extra arguments name with --model, which comes after remuda's own and wins; "" for none
export function modelIn(args: string): string {
  const named = [...unescaped(args).matchAll(/--model(?:=|\s+)["']?([^\s"']+)/g)];
  return named.length ? named[named.length - 1]![1]! : "";
}

// applies says whether an agent on this model, in fast mode or not, is on the retirement's way
export const applies = (r: Retirement, agent: string, model: string, fast: boolean): boolean =>
  (agent || "claude") === r.agent && canonical(model) === r.model && (!r.mode || (r.mode === "fast" && fast));

// endsOn is the retirement's day as a local date, for the words: the table's day, wherever the person is
const endsOn = (r: Retirement): Date => {
  const [y, m, d] = r.on.split("-").map(Number);
  return new Date(y!, (m ?? 1) - 1, d ?? 1);
};

// failingOn says whether the retirement's day has come, the start of that day in UTC as the server reckons it
export function failingOn(r: Retirement, now: number): boolean {
  const [y, m, d] = r.on.split("-").map(Number);
  const ends = Date.UTC(y!, (m ?? 1) - 1, d ?? 1);
  return Number.isFinite(ends) && now >= ends;
}

// retiredFor is the retirement a form's setting is on the way of, weighed on its machine or, for any machine, on each
// one with Claude Code; null when none
export function retiredFor(table: Retirement[], s: ModelSetting, defaults: Record<string, Defaults>): Found | null {
  const where = s.machine ? [s.machine] : Object.keys(defaults).length ? Object.keys(defaults) : [""];
  for (const r of table) {
    let found: Found | null = null;
    for (const machine of where) {
      const d = defaults[machine] ?? {};
      const fromArgs = modelIn(s.args);
      const model = fromArgs || s.model.trim() || d.model || "";
      const said = fastIn(s.args);
      const fast = said ?? !!d.fast;
      if (!applies(r, s.agent, model, fast)) continue;
      found ??= { retirement: r, model: fromArgs ? "arguments" : s.model.trim() ? "field" : "default", fast: !r.mode ? "" : said !== undefined ? "arguments" : "default", machines: [] };
      if (machine) found.machines.push(machine);
    }
    if (found) return found;
  }
  return null;
}

// defaultsOf is every machine's Claude Code defaults from its vitals; a machine without Claude Code has none
export function defaultsOf(machines: { id: string; health?: { claude?: string; claude_model?: string; claude_fast?: boolean } }[]): Record<string, Defaults> {
  const out: Record<string, Defaults> = {};
  for (const m of machines) if (m.health?.claude) out[m.id] = { model: m.health.claude_model, fast: m.health.claude_fast };
  return out;
}

// whatGoes is a retirement as a person reads it: "Fast mode on Opus 4.8", or the model itself
const whatGoes = (r: Retirement): string => (r.mode === "fast" ? t("Fast mode on {model}", { model: r.name }) : r.name);

// noticeWords is a form's notice: when it goes or that it went, why it applies when the machine's own settings make it
// so (names are the machines' labels), and what to use unless the form offers a button that puts it in the field
export function noticeWords(found: Found, now: number, labels: string[] = [], offered = false): string {
  const r = found.retirement;
  const vars = { what: whatGoes(r), date: fmtDate(endsOn(r)), replacement: r.replacement_name };
  let words = failingOn(r, now) ? t("{what} was removed on {date}: its requests fail now and do not fall back.", vars) : t("{what} ends on {date}: after that its requests fail and do not fall back.", vars);
  const where = labels.length ? listWords(labels) : "";
  if (where && found.model === "default") words += " " + t("{model} is the default model in Claude Code's settings on {machines}.", { model: r.name, machines: where });
  if (where && found.fast === "default") words += " " + t("Fast mode is saved on in Claude Code's settings on {machines}.", { machines: where });
  if (!offered) words += " " + t("Use {replacement}.", vars);
  return words;
}

const KINDS: { kind: RetiringHit["kind"]; words: (n: number) => string }[] = [
  { kind: "recipe", words: (n) => tn(n, "{n} recipe", "{n} recipes") },
  { kind: "schedule", words: (n) => tn(n, "{n} agent schedule", "{n} agent schedules") },
  { kind: "project", words: (n) => tn(n, "the agents of {n} project", "the agents of {n} projects") },
  { kind: "stage", words: (n) => tn(n, "{n} pipeline stage", "{n} pipeline stages") },
  { kind: "pane", words: (n) => tn(n, "{n} running agent", "{n} running agents") },
];

// retiringConcerns are a machine's hits as the ⓘ's concerns, one per retirement: what is set to it there, counted by
// kind, and what to move to; bad once its day has come
export function retiringConcerns(hits: RetiringHit[] | undefined, table: Retirement[], now: number): { key: string; text: string; bad: boolean }[] {
  const groups = new Map<string, { r: Retirement; group: RetiringHit[] }>();
  for (const hit of hits ?? []) {
    const r = retirementOf(table, hit);
    if (!r) continue;
    const key = `${hit.model}|${hit.mode ?? ""}`;
    const entry = groups.get(key) ?? { r, group: [] };
    entry.group.push(hit);
    groups.set(key, entry);
  }
  return [...groups.entries()].map(([key, { r, group }]) => {
    const counts = KINDS.map(({ kind, words }) => ({ n: new Set(group.filter((h) => h.kind === kind).map((h) => h.id)).size, words })).filter((c) => c.n > 0);
    const vars = { what: whatGoes(r), date: fmtDate(endsOn(r)), set: listWords(counts.map((c) => c.words(c.n))), replacement: r.replacement_name };
    const bad = failingOn(r, now);
    const text = bad
      ? t("{what} was removed on {date} and is still set for {set} here: their requests fail now — move to {replacement}", vars)
      : t("{what} ends on {date}, set for {set} here: move to {replacement}", vars);
    return { key: `retiring-${key}`, text, bad };
  });
}

// listWords joins words as a sentence does: "a, b and c"
function listWords(parts: string[]): string {
  const rest = [...parts];
  const last = rest.pop() ?? "";
  return rest.length ? t("{names} and {last}", { names: rest.join(", "), last }) : last;
}
