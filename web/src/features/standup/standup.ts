// The pure part of the standup: the period's start, the order of the agents, the lines a recap reads as, the Markdown of one.

import { shownPath } from "@/features/chat/footprint";
import { tok, usd } from "@/features/usage/format";
import { plural } from "@/lib/text";
import { spent } from "@/lib/time";
import type { Item, Recap } from "./api";
import { t } from "@/lib/i18n";

export type Period = "today" | "day" | "week";

export const PERIODS: { value: Period; label: string }[] = [
  { value: "today", label: t("Since midnight") },
  { value: "day", label: t("Last 24 h") },
  { value: "week", label: t("Last 7 days") },
];

// sinceFor is the period's start as RFC 3339, or nothing for "today": the server cuts at midnight in the device's zone
export function sinceFor(period: Period, now = Date.now()): string | undefined {
  if (period === "today") return undefined;
  const hours = period === "day" ? 24 : 24 * 7;
  return new Date(now - hours * 3600_000).toISOString();
}

// quiet is a recap with nothing in it
export const quiet = (r: Recap) => r.prompts === 0 && r.answers === 0 && r.calls === 0;

const RANK: Record<string, number> = { blocked: 0, done: 1, working: 2 };

// order puts the agents that need the person first, then the finished, then the working, the dearest first within a rank
export function order(items: Item[]): Item[] {
  const rank = (item: Item) => (item.alert ? 0 : (RANK[item.state] ?? 3));
  return [...items].sort((a, b) => rank(a) - rank(b) || b.recap.cost - a.recap.cost || a.workspace.localeCompare(b.workspace));
}

// stateLabel is the item's state in a word or two
export function stateLabel(item: Item): string {
  if (item.alert) return "needs you";
  switch (item.state) {
    case "blocked":
      return "needs you";
    case "done":
      return "finished";
    case "working":
      return "working";
    case "idle":
      return "idle";
  }
  return "";
}

// where names the agent's place: its workspace, and the machine when it is not this one
export const where = (item: Item) => (item.machine !== "local" && item.label ? t("{workspace} on {machine}", { workspace: item.workspace, machine: item.label }) : item.workspace);

// numbers is the one line of figures: prompts, answers, time, tokens, cost
export function numbers(r: Recap): string {
  const parts = [plural(r.prompts, "prompt"), plural(r.answers, "answer")];
  if (r.took > 0) parts.push(spent(r.took));
  if (r.tokens > 0) parts.push(t("{n} tokens", { n: tok(r.tokens) }));
  if (r.cost > 0) parts.push(usd(r.cost));
  return parts.join(" · ");
}

// calls is the tools line: how many, the busiest, the failures, the longest
export function calls(r: Recap): string {
  const busiest = Object.entries(r.tools)
    .sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]))
    .slice(0, 4)
    .map(([name, n]) => `${name} ${n}`);
  let line = plural(r.calls, "tool call");
  if (busiest.length) line += ` (${busiest.join(", ")})`;
  if (r.failed > 0) line += ` · ${plural(r.failed, "failed call")}`;
  if (r.longest) line += ` · ${t("longest {call} {time}", { call: r.longest.name, time: spent(r.longest.took) })}${r.longest.hint ? ` (${r.longest.hint})` : ""}`;
  return line;
}

// fileList names the first few files relative to the folder, "+N more" for the rest
export function fileList(paths: string[], dir: string, max = 6): string {
  const shown = paths.slice(0, max).map((path) => shownPath(path, dir));
  if (paths.length > max) shown.push(t("+{n} more", { n: paths.length - max }));
  return shown.join(", ");
}

// recapMarkdown is one conversation's recap ready to paste, the lines the standup report uses
export function recapMarkdown(title: string, r: Recap, dir: string): string {
  const code = (texts: string[], max: number) => {
    const out = texts.slice(0, max).map((text) => `\`${shownPath(text, dir).replace(/`/g, "'")}\``);
    if (texts.length > max) out.push(`+${texts.length - max} more`);
    return out.join(", ");
  };
  const lines = [`## ${title}`, "", `- ${numbers(r)}`];
  if (r.calls > 0) lines.push(`- ${calls(r)}`);
  if (r.commands.length) lines.push(`- Ran: ${code(r.commands.map((command) => (command.length > 60 ? command.slice(0, 59) + "…" : command)), 6)}`);
  if (r.edited.length) lines.push(`- Edited: ${code(r.edited, 12)}`);
  if (r.read.length) lines.push(`- Read: ${code(r.read, 12)}`);
  const more = [r.subagents > 0 ? plural(r.subagents, "subagent") : "", r.questions > 0 ? plural(r.questions, "question") : ""].filter(Boolean);
  if (more.length) lines.push(`- ${more.join(" · ")}`);
  if (r.said) lines.push(`- Last answer: ${r.said}`);
  return lines.join("\n") + "\n";
}
