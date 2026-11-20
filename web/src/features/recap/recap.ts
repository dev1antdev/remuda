// The pure part of one conversation's recap: the lines it reads as on the page, and its Markdown.

import { shownPath } from "@/features/chat/footprint";
import { tok, usd } from "@/features/usage/format";
import { plural } from "@/lib/text";
import { spent } from "@/lib/time";
import type { Recap } from "./api";
import { t } from "@/lib/i18n";

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

// recapMarkdown is one conversation's recap ready to paste
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
