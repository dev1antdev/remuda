// The switch "Don't wait for permissions: deny and carry on" on what starts an agent with nobody watching — an agent
// schedule, a project's queue and pipeline, a second opinion: which agents know the setting (Claude Code's
// permissions.denyInsteadOfPrompting), what the switch says for the others, and what a new record starts with. The
// server decides what reaches the command line (internal/sessions: deny.go).

import { t, tn } from "@/lib/i18n";

// Denial is a tool call Claude Code refused without asking, nobody being there to answer its permission prompt
export type Denial = { tool: string; what?: string; at?: string; tool_id?: string };

// canDenyPrompts says whether remuda can start the agent so it denies what would wait on a permission prompt
export const canDenyPrompts = (agent: string): boolean => (agent || "claude") === "claude";

// denyDefault is the switch a form opens with: on for a record being made, else what the record was saved with — off
// for one saved before the switch existed
export const denyDefault = (saved: boolean, value: boolean | undefined): boolean => (saved ? value === true : (value ?? true));

// whyNoDeny is what the switch says, disabled, when none of the agents it covers knows the setting; "" when one does
export function whyNoDeny(agents: { id: string; label: string }[]): string {
  if (agents.length === 0 || agents.some((a) => canDenyPrompts(a.id))) return "";
  const names = [...new Set(agents.map((a) => a.label))].join(", ");
  return t("Only Claude Code can deny instead of asking: {agents} would wait at its prompts.", { agents: names });
}

// deniedCount is how many calls were denied, in words
export const deniedCount = (n: number): string => tn(n, "{n} call denied", "{n} calls denied");

// denialLine is one refused call as a list names it: the tool and what it was about
export const denialLine = (d: Denial): string => (d.what ? `${d.tool} · ${d.what}` : d.tool);

// deniedList is the calls on one line, the first few named and the rest counted
export function deniedList(calls: Denial[], named = 3): string {
  const names = calls.slice(0, named).map(denialLine);
  if (calls.length > named) names.push(t("+{n} more", { n: calls.length - named }));
  return names.join("; ");
}

// denyHint is what the switch means, under it
export const denyHint = (): string =>
  t(
    "Claude Code denies any call that would wait for your permission and goes on with the rest; what it was denied is listed once it has answered. In its default mode every edit asks first, so for the agent to change files add --permission-mode acceptEdits or auto to the extra arguments, or allow rules in its settings.",
  );
