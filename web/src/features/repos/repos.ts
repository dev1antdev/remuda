// The Repositories place's rows, the pure part: what a row says of its upstream, which of fetch, pull and push its
// menu offers first, the groups by folder, and how old the last fetch is. After Manygit's rows (rabeeh-ta's git-graph).

import { t, tn } from "@/lib/i18n";
import type { Session } from "@/features/roster/api";
import { folderOf } from "@/features/roster/workspaces";
import type { Listed, Repo } from "./api";
import type { SyncAction } from "./syncApi";

// SyncKind is how a row stands against its upstream
export type SyncKind = "reading" | "error" | "failed" | "detached" | "unpushed" | "local" | "gone" | "diverged" | "ahead" | "behind" | "ok";

// syncOf is what a row says beside its name: "…" while it is read, never 0; a fetch that failed says so
export function syncOf(repo: Repo | null | undefined, busy = false, fetchFailed = ""): SyncKind {
  if (!repo || busy) return "reading";
  if (fetchFailed) return "failed";
  if (repo.error) return "error";
  if (repo.detached) return "detached";
  if (!repo.upstream) return repo.remote ? "unpushed" : "local";
  if (repo.gone) return "gone";
  if (repo.ahead && repo.behind) return "diverged";
  if (repo.ahead) return "ahead";
  if (repo.behind) return "behind";
  return "ok";
}

// syncLabel is the word a row shows for a state with no counts, "" where the counts or nothing say it
export function syncLabel(kind: SyncKind): string {
  switch (kind) {
    case "reading":
      return "…";
    case "failed":
      return t("fetch failed");
    case "error":
      return t("can't read");
    case "unpushed":
      return t("not pushed");
    case "local":
      return t("no remote");
    case "gone":
      return t("upstream gone");
  }
  return "";
}

// syncTip is the row's state in a sentence, for its tooltip: better-changes' "N not pulled"
export function syncTip(repo: Repo, kind: SyncKind): string {
  const pushed = tn(repo.ahead, "{n} commit not pushed", "{n} commits not pushed");
  const pulled = tn(repo.behind, "{n} not pulled", "{n} not pulled");
  switch (kind) {
    case "diverged":
      return t("{pushed}, {pulled}: it has diverged from {upstream}", { pushed, pulled, upstream: repo.upstream ?? "" });
    case "ahead":
      return t("{pushed} to {upstream}", { pushed, upstream: repo.upstream ?? "" });
    case "behind":
      return t("{pulled} from {upstream}", { pulled, upstream: repo.upstream ?? "" });
    case "ok":
      return t("In step with {upstream}", { upstream: repo.upstream ?? "" });
    case "unpushed":
      return t("The branch is on no remote yet: Push sends it and sets its upstream");
    case "local":
      return t("No remote: nothing to fetch, pull or push");
    case "gone":
      return t("Its upstream {upstream} is no longer on the remote", { upstream: repo.upstream ?? "" });
    case "detached":
      return t("HEAD is detached: no branch to pull or push");
  }
  return "";
}

// dirtyWords is the uncommitted count in words, for the dot's tooltip
export function dirtyWords(repo: Repo): string {
  const parts = [tn(repo.dirty, "{n} uncommitted change", "{n} uncommitted changes")];
  if (repo.staged) parts.push(tn(repo.staged, "{n} staged", "{n} staged"));
  if (repo.conflicts) parts.push(tn(repo.conflicts, "{n} in conflict", "{n} in conflict"));
  return parts.join(" · ");
}

// Offer is one of fetch, pull and push as a row's menu lists it
export type Offer = { action: SyncAction; label: string };

// syncOffers is fetch, pull and push in the order the row's ↑ ↓ suggests: behind pulls first, ahead pushes first, a
// branch with no upstream pushes; only what can run there
export function syncOffers(repo: Repo): Offer[] {
  if (!repo.remote || repo.error) return [];
  const branch = !!repo.branch && !repo.detached;
  const pull: Offer | null = branch && repo.upstream && !repo.gone ? { action: "pull", label: t("Pull") } : null;
  const push: Offer | null = branch ? { action: "push", label: repo.upstream && !repo.gone ? t("Push") : t("Push and set upstream") } : null;
  const fetch: Offer = { action: "fetch", label: t("Fetch") };
  const order = repo.behind && !repo.ahead ? [pull, fetch, push] : repo.ahead && !repo.behind ? [push, fetch, pull] : !repo.upstream ? [push, fetch] : [fetch, pull, push];
  return order.filter((offer): offer is Offer => offer !== null);
}

// fetchable: a row Fetch all reaches, one with a remote git could read
export const fetchable = (repo: Repo): boolean => repo.remote && !repo.error;

// Group is the repositories of one folder of a machine
export type Group = { parent: string; repos: Repo[] };

// groupsOf lays a machine's repositories out by the folder they sit in, in the order the server sorted them
export function groupsOf(repos: Repo[]): Group[] {
  const groups: Group[] = [];
  for (const repo of repos) {
    const last = groups[groups.length - 1];
    if (last && last.parent === repo.parent) last.repos.push(repo);
    else groups.push({ parent: repo.parent, repos: [repo] });
  }
  return groups;
}

// keyOf is a repository's key across the machines
export const keyOf = (machineId: string, path: string): string => `${machineId}|${path}`;

// machineWords is what a machine's head says under its name
export function machineWords(listed: Listed | undefined): string {
  if (!listed) return "…";
  if (listed.away === "connecting") return t("connecting…");
  if (listed.away) return t("away · {why}", { why: listed.away });
  if (listed.error) return t("could not be read · {why}", { why: listed.error });
  return tn(listed.repos.length, "{n} repository", "{n} repositories");
}

// scanWords says what the machine scans, for its head
export function scanWords(listed: Listed): string {
  if (!listed.scan) return "";
  const where = t("{folder}, {n} deep", { folder: listed.scan_where || listed.scan.folder, n: listed.scan.depth });
  if (listed.scan_missing) return t("scans {where}: the folder is not there", { where });
  if (listed.more) return t("scans {where}: the first 500 found", { where });
  return t("scans {where}", { where });
}

// workspaceOn is the workspace standing on the repository, the sidebar's way (its folder), for Open Changes; never the
// floating terminal's or the concierge's
export function workspaceOn(sessions: Session[], path: string): Session | undefined {
  return sessions.find((session) => !session.scratch && !session.concierge && folderOf(session) === path);
}

// paneOf is the pane a workspace opens on: its active tab's agent, else any agent's, else its first pane
export function paneOf(session: Session): string | undefined {
  const active = session.tabs.find((tab) => tab.id === session.active_tab_id);
  const panes = [...(active?.panes ?? []), ...session.tabs.flatMap((tab) => tab.panes)];
  return (panes.find((pane) => pane.agent) ?? panes[0])?.id;
}

// replaced is the machine's rows with one repository read again after an action
export function replaced(machines: Listed[] | null, machineId: string, repo: Repo): Listed[] | null {
  if (!machines) return machines;
  return machines.map((m) => (m.id === machineId ? { ...m, repos: m.repos.map((r) => (r.path === repo.path ? { ...repo, parent: r.parent, workspace: r.workspace } : r)) } : m));
}
