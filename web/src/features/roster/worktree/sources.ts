// What the New worktree dialog lists, after Orca's: a repository's branches — checked out already, local, only on
// origin — its open issues and pull requests, and a pasted number or link that names one. Pure, so the rows, the
// sentences and the jumps are tested without a page.

import { basename } from "@/lib/text";
import { t } from "@/lib/i18n";
import type { Machine, Session } from "../api";

export type Branch = { name: string; at: number };
export type Checkout = { path: string; branch?: string; main?: boolean };
export type Team = { file: string; script: string };
// Source is what a repository offers a new worktree, read on its machine (GET /api/worktrees/source)
export type Source = { root: string; name: string; origin: string; github: string; base: string; local: Branch[]; remote: Branch[]; checkouts: Checkout[]; team?: Team };
export type Issue = { number: number; title: string };
export type PR = { number: number; title: string; head: string; cross_repo?: boolean; closes?: number[] };
// Lists are origin's open issues and pull requests through gh, or why there are none to show
export type Lists = { issues: Issue[]; prs: PR[]; trouble?: string };

export type Mode = "branch" | "issue" | "pr" | "new";
export const MODES: Mode[] = ["branch", "issue", "pr", "new"];

// Item is one row of the list; Tag says what kind of source it is
export type Tag = "Open" | "Local" | "Remote" | "Issue" | "PR" | "GitHub";
export type Item =
  | { key: string; kind: "open"; branch: string; path: string; label: string; sub: string; tag: "Open"; mono: true }
  | { key: string; kind: "local"; branch: string; label: string; sub: string; tag: "Local"; mono: true }
  | { key: string; kind: "remote"; branch: string; label: string; sub: string; tag: "Remote"; mono: true }
  | { key: string; kind: "issue"; number: number; title: string; pr?: PR; label: string; sub: string; tag: "Issue"; mono?: false }
  | { key: string; kind: "pr"; number: number; title: string; head: string; cross_repo?: boolean; label: string; sub: string; tag: "PR"; mono?: false }
  | { key: string; kind: "lookup"; want: "issue" | "pr"; number: number; label: string; sub: string; tag: "GitHub"; mono?: false };

// tilde is a path under the home said with ~
export function tilde(path: string, home: string): string {
  const p = path.replace(/\\/g, "/");
  const h = home.replace(/\\/g, "/").replace(/\/+$/, "");
  return h && (p === h || p.startsWith(h + "/")) ? `~${p.slice(h.length)}` : path;
}

// Ref is what a pasted #123, 123 or GitHub link names
export type Ref = { number: number; kind?: "issue" | "pr" };

export function parseRef(text: string): Ref | null {
  const link = /github\.com\/[\w.-]+\/[\w.-]+\/(pull|issues)\/(\d+)/.exec(text);
  if (link) return { number: Number(link[2]), kind: link[1] === "pull" ? "pr" : "issue" };
  const number = /^#?(\d+)$/.exec(text.trim());
  return number ? { number: Number(number[1]) } : null;
}

// jumpFor is the tab a pasted reference on the Branch or New tab belongs on, null to stay
export function jumpFor(mode: Mode, text: string, lists: Lists | null): Mode | null {
  if (mode !== "branch" && mode !== "new") return null;
  const ref = parseRef(text.trim());
  if (!ref || (!ref.kind && !text.trim().startsWith("#"))) return null;
  if (ref.kind) return ref.kind;
  if (lists?.prs.some((p) => p.number === ref.number)) return "pr";
  if (lists?.issues.some((i) => i.number === ref.number)) return "issue";
  return null;
}

// prFor is the open pull request that works on the issue: one GitHub links to it, one that says it closes it, or one on its branch
export function prFor(prs: PR[], issue: number, branch: string): PR | undefined {
  return prs.find((p) => (p.closes ?? []).includes(issue)) ?? prs.find((p) => p.head === branch);
}

const MAX_ROWS = 80;

// items are the rows a tab lists for what is typed; the New tab lists none
export function items(mode: Mode, query: string, src: Source | null, lists: Lists | null, home: string, branchFor: (issue: Issue) => string): Item[] {
  if (!src || mode === "new") return [];
  const q = query.trim().toLowerCase();
  const has = (s: string) => !q || s.toLowerCase().includes(q);
  if (mode === "branch") {
    const out: Item[] = [];
    const checkedOut = new Set(src.checkouts.map((c) => c.branch).filter(Boolean));
    for (const c of src.checkouts) if (!c.main && c.branch && has(c.branch)) out.push({ key: `open:${c.path}`, kind: "open", branch: c.branch, path: c.path, label: c.branch, sub: tilde(c.path, home), tag: "Open", mono: true });
    for (const b of src.local) if (!checkedOut.has(b.name) && has(b.name)) out.push({ key: `local:${b.name}`, kind: "local", branch: b.name, label: b.name, sub: t("Local branch"), tag: "Local", mono: true });
    const locals = new Set(src.local.map((b) => b.name));
    for (const b of src.remote) if (!locals.has(b.name) && has(b.name)) out.push({ key: `remote:${b.name}`, kind: "remote", branch: b.name, label: b.name, sub: t("origin/{branch} · not checked out here", { branch: b.name }), tag: "Remote", mono: true });
    return out.slice(0, MAX_ROWS);
  }
  const prs = lists?.prs ?? [];
  const issueItem = (i: Issue): Item => {
    const pr = prFor(prs, i.number, branchFor(i));
    return { key: `issue:${i.number}`, kind: "issue", number: i.number, title: i.title, pr, label: i.title, sub: pr ? t("#{n} · PR #{pr} works on it", { n: i.number, pr: pr.number }) : `#${i.number}`, tag: "Issue" };
  };
  const prItem = (p: PR): Item => ({ key: `pr:${p.number}`, kind: "pr", number: p.number, title: p.title, head: p.head, cross_repo: p.cross_repo, label: p.title, sub: `#${p.number} · ${p.head}`, tag: "PR" });
  const ref = parseRef(query.trim());
  if (mode === "pr") {
    if (ref) {
      const hit = prs.find((p) => p.number === ref.number);
      return [hit ? prItem(hit) : lookupItem("pr", ref.number)];
    }
    return prs.filter((p) => has(p.title) || has(`#${p.number}`) || has(p.head)).map(prItem);
  }
  const issues = lists?.issues ?? [];
  if (ref) {
    const hit = issues.find((i) => i.number === ref.number);
    return [hit ? issueItem(hit) : lookupItem("issue", ref.number)];
  }
  return issues.filter((i) => has(i.title) || has(`#${i.number}`)).map(issueItem);
}

function lookupItem(want: "issue" | "pr", number: number): Item {
  return { key: `lookup:${want}:${number}`, kind: "lookup", want, number, label: `#${number}`, sub: t("Not in the open list · look it up on GitHub"), tag: "GitHub" };
}

// emptyWords is what the list says when it has no row
export function emptyWords(mode: Mode, src: Source | null, reading: boolean, error: string, lists: Lists | null): string {
  if (error) return error;
  if (reading || !src) return t("Reading branches…");
  if (mode === "issue" || mode === "pr") {
    if (!src.github) return t("This repository's origin isn't on GitHub.");
    if (!lists) return t("Loading from GitHub…");
    if (lists.trouble) return lists.trouble;
  }
  return t("Nothing matches.");
}

// Repository is one the machine has a workspace in: its main folder, its name and the workspace a worktree branches off
export type Repository = { root: string; name: string; workspace: string; repoName: string };

// repositoriesOf is every repository among the machine's workspaces, the one asked for first
export function repositoriesOf(machine: Machine, first: string): Repository[] {
  const byRoot = new Map<string, Session[]>();
  for (const s of machine.sessions) {
    if (s.scratch || s.concierge || !(s.worktree || s.branch)) continue;
    const root = s.folder || s.dir;
    if (!root) continue;
    byRoot.set(root, [...(byRoot.get(root) ?? []), s]);
  }
  const out: Repository[] = [];
  for (const [root, sessions] of byRoot) {
    const main = sessions.find((s) => !s.worktree?.linked && s.dir === root) ?? sessions[0]!;
    out.push({ root, name: basename(root), workspace: main.id, repoName: main.worktree?.repo_name || basename(root) });
  }
  return out.sort((a, b) => Number(b.root === first) - Number(a.root === first) || a.name.localeCompare(b.name));
}

// baseChoices are the starting points a new branch may take: the default first, origin's, then the local branches
export function baseChoices(src: Source): string[] {
  return [...new Set([src.base, ...src.remote.map((b) => `origin/${b.name}`), ...src.local.map((b) => b.name)].filter(Boolean))];
}
