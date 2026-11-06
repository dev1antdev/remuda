// A conversation forked into a new tab: which agents can start on a copy of their conversation — the same kinds as the
// server's sessions.ForkArgs — and what the menus and the toasts say. The pure part; forkPane.ts runs it.

import { VENDOR_NAME } from "@/ui/brand";
import { t } from "@/lib/i18n";

// FORKABLE is every agent whose own CLI starts on a copy of a session: Claude Code's --fork-session, Codex's fork
export const FORKABLE: ReadonlySet<string> = new Set(["claude", "codex"]);

export const canFork = (agent: string | undefined): boolean => !!agent && FORKABLE.has(agent);

// forkLabel is the menu row: "Fork Claude Code into a new tab"
export const forkLabel = (agent: string): string => t("Fork {agent} into a new tab", { agent: VENDOR_NAME[agent] ?? agent });

// forkedWords is the toast once the fork runs, or the reason the tab came up without its agent
export function forkedWords(agent: string, agentError?: string): { text: string; error: boolean } {
  const name = VENDOR_NAME[agent] ?? agent;
  if (agentError) return { text: t("The tab is open, but {agent} did not start on the copy: {error}", { agent: name, error: agentError }), error: true };
  return { text: t("Forked: {agent} goes on from a copy of this conversation in a new tab", { agent: name }), error: false };
}
