// A conversation's subagents on the page: which of them a turn spawned, whether a poll changed what a row shows, the
// words of a row and of a tile's state line. Pure functions over the chat answer, tested beside.

import { plural } from "@/lib/text";
import type { Subagent, Turn } from "./api";
import { toolLabel } from "./footprint";
import { firstLine } from "./said";
import { t } from "@/lib/i18n";

// the tools that spawn a subagent: Agent today, Task in older transcripts
export const SPAWN_TOOLS = new Set(["Agent", "Task"]);

// subagentsOf is the subagents the turn's Agent calls spawned, in the turn's order
export function subagentsOf(turn: Turn, all: Subagent[] | undefined): Subagent[] {
  if (!all?.length) return [];
  const out: Subagent[] = [];
  for (const part of turn.parts) {
    if (part.type !== "tool" || !part.tool_id) continue;
    for (const s of all) if (s.tool_id === part.tool_id) out.push(s);
  }
  return out;
}

// sameSubagents says whether two readings show the same rows, so a poll that moved nothing redraws nothing
export function sameSubagents(a: Subagent[], b: Subagent[]): boolean {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) {
    const x = a[i]!;
    const y = b[i]!;
    if (x.id !== y.id || x.status !== y.status || x.tools !== y.tools || x.failed !== y.failed || x.report !== y.report || x.description !== y.description) return false;
    if (subagentLine(x) !== subagentLine(y)) return false;
  }
  return true;
}

// subagentLine is the middle of a row: the tool call the agent is on, else the first line of what it answered
export function subagentLine(s: Subagent): string {
  if (s.status !== "done" && s.doing) {
    const { name, hint } = toolLabel(s.doing);
    return hint ? `${name} · ${hint}` : name;
  }
  return s.report ? firstLine(s.report) : "";
}

// subagentWords is what a tile's state line says of them: the lead waiting on them, or how many still work
export function subagentWords(subs: Subagent[] | undefined, waiting: boolean): string {
  const working = subs?.filter((s) => s.status === "working").length ?? 0;
  if (waiting) return working > 0 ? t("waiting on {subagents}", { subagents: plural(working, "subagent") }) : t("waiting on a subagent");
  return working > 0 ? t("{subagents} working", { subagents: plural(working, "subagent") }) : "";
}
