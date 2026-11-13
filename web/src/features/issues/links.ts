// Which workspace an issue already has: one whose branch the checkout's config links to it (git config
// branch.<b>.remuda-issue owner/repo#N) or whose branch is named after it (issue-12-…, dev/feat/12, feat/12-…), among
// the checkouts of its repository on the machine. And where a new worktree for it would branch off: the main checkout.

import type { Machine, Session } from "@/features/roster/api";
import type { Repo } from "./api";

// namedAfter says whether a branch's name carries the issue's number as a word of its own
export function namedAfter(branch: string, number: number): boolean {
  return new RegExp(`(?:^|/)(?:issue-|gh-)?${number}(?:$|[-/])`).test(branch);
}

const inRepo = (session: Session, repo: Repo) => !!session.dir && repo.dirs.includes(session.dir) && !session.scratch && !session.concierge;

// openWorkspace is the workspace working on the issue: a linked branch first, then one named after it
export function openWorkspace(machine: Machine | undefined, repo: Repo | undefined, number: number): Session | null {
  if (!machine || !repo) return null;
  const candidates = machine.sessions.filter((s) => inRepo(s, repo) && s.branch);
  const linked = new Set(repo.links.filter((l) => l.number === number).map((l) => l.branch));
  return candidates.find((s) => linked.has(s.branch!)) ?? candidates.find((s) => namedAfter(s.branch!, number)) ?? null;
}

// startFrom is the workspace a new worktree for the issue branches off: the main checkout, else any of the repository
export function startFrom(machine: Machine | undefined, repo: Repo | undefined): Session | null {
  if (!machine || !repo) return null;
  const there = machine.sessions.filter((s) => inRepo(s, repo));
  return there.find((s) => !s.worktree?.linked && s.dir === repo.folder) ?? there.find((s) => !s.worktree?.linked) ?? there[0] ?? null;
}

// paneOf is the pane to open in a workspace: its active tab's first agent, else that tab's first pane
export function paneOf(session: Session): string {
  const tab = session.tabs.find((x) => x.id === session.active_tab_id) ?? session.tabs[0];
  return (tab?.panes.find((p) => p.agent) ?? tab?.panes[0])?.id ?? "";
}
