// What Create will do, from the Branch field as it stands, said before it is pressed (Orca's live note): open the
// checkout that has the branch, check a local one out, track one only on origin, fetch a pull request's head, or make
// a new branch from the base. The server works the same plan out again from a fresh read (worktrees.MakePlan).

import { t } from "@/lib/i18n";
import { tilde, type Item, type Source } from "./sources";

// Pick is what the Details were filled from, as the create route takes it
export type Pick = { kind: "branch" | "issue" | "pr" | "new"; branch?: string; number?: number; head?: string; pr?: number; cross_repo?: boolean };

export type Plan =
  | { action: "open"; path: string; note: string }
  | { action: "local" | "remote"; note: string; fetch?: boolean }
  | { action: "new"; from: string; fromBase: boolean; folder: string; note: string }
  | { action: "error"; note: string };

const BAD_BRANCH = /[\s~^:?*[\\]|\.\.|@\{|\/\/|^[-/.]|[/.]$|\.lock$/;

// badBranch says git would refuse the name, as far as a name typed goes (check-ref-format's rules)
export const badBranch = (name: string): boolean => !name || name === "@" || BAD_BRANCH.test(name) || /[\x00-\x1f\x7f]/.test(name);

// slug is a branch as herdr names its worktree's folder: lower case, a dash for every other run of characters, and
// "worktree" for one with no a-z or 0-9 in it (a Cyrillic branch), as the server's worktrees.Slug
export function slug(branch: string): string {
  const name = branch
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "");
  return name || "worktree";
}

// titleSlug is an issue's title as a branch's word, as the server's repos.TitleSlug makes it
export function titleSlug(title: string): string {
  return title
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 40)
    .replace(/-+$/, "");
}

// fill puts a template's {number} {title} {repo} {branch} {slug} in place
export function fill(template: string, number: number, title: string, repo: string, branch: string): string {
  return template.replaceAll("{number}", String(number)).replaceAll("{title}", title).replaceAll("{repo}", repo).replaceAll("{branch}", branch).replaceAll("{slug}", titleSlug(title));
}

// forkBranch is the branch a fork's pull request is fetched into, as the server's worktrees.ForkBranch names it: the
// head's name on the fork says nothing of this repository's branches (a fork's main is not this main)
export const forkBranch = (number: number, head: string): string => `pr-${number}-${head}`;

// prBranch is the branch a pull request's worktree checks out: its head, or for a fork's a branch of its own
const prBranch = (pr: { number: number; head: string; cross_repo?: boolean }): string => (pr.cross_repo ? forkBranch(pr.number, pr.head) : pr.head);

// pickOf is the source a row stands for, and the branch and workspace name its pick fills the Details with
export function pickOf(item: Item, issueBranch: (number: number, title: string) => string): { pick: Pick; branch: string; name: string } {
  switch (item.kind) {
    case "open":
    case "local":
    case "remote":
      return { pick: { kind: "branch", branch: item.branch }, branch: item.branch, name: "" };
    case "issue":
      return {
        pick: { kind: "issue", number: item.number, pr: item.pr?.number, head: item.pr?.head, cross_repo: item.pr?.cross_repo },
        branch: item.pr ? prBranch(item.pr) : issueBranch(item.number, item.title),
        name: item.title,
      };
    case "pr":
      return { pick: { kind: "pr", number: item.number, head: item.head, cross_repo: item.cross_repo }, branch: prBranch(item), name: item.title };
    case "lookup":
      return { pick: { kind: item.want, number: item.number }, branch: "", name: "" };
  }
}

// Where is what the plan needs to say where a new worktree goes: the machine's home, herdr's root, the project's location
export type Where = { home: string; root: string; location: string };

// folderFor is where a new worktree of the branch goes on its machine
export function folderFor(src: Source, where: Where, branch: string): string {
  const name = slug(branch);
  const sep = where.root.includes("\\") && !where.root.includes("/") ? "\\" : "/";
  return where.location ? `${where.location.replace(/[\\/]+$/, "")}${sep}${name}` : `${where.root}${sep}${src.name}${sep}${name}`;
}

// planOf is what Create will do with the branch as typed, from the pick and the base
export function planOf(src: Source, branch: string, base: string, pick: Pick, where: Where): Plan {
  const b = branch.trim();
  if (!b) return { action: "error", note: t("Name the branch.") };
  if (badBranch(b)) return { action: "error", note: t("“{branch}” isn't a valid branch name.", { branch: b }) };
  const open = src.checkouts.find((c) => c.branch === b);
  if (open) return { action: "open", path: open.path, note: t("Already checked out at {path} · opens it", { path: tilde(open.path, where.home) }) };
  const local = (name: string) => src.local.some((x) => x.name === name);
  const remote = (name: string) => src.remote.some((x) => x.name === name);
  if (local(b)) return { action: "local", note: pick.kind === "issue" ? t("{branch} already exists · checks it out", { branch: b }) : t("Local branch · checks it out") };
  if (remote(b)) return { action: "remote", note: t("Only on origin · makes a local branch tracking origin/{branch}", { branch: b }) };
  if (forkHead(pick, b)) return { action: "local", fetch: true, note: t("From a fork · fetches pull/{n}/head as {branch}", { n: prOf(pick)!, branch: b }) };
  if (headPicked(pick) && b === pick.head) return { action: "remote", fetch: true, note: t("Fetches {branch} from origin, then tracks it", { branch: b }) };
  let from = "";
  if (pick.kind === "branch" && pick.branch && local(pick.branch)) from = pick.branch;
  else if (pick.kind === "branch" && pick.branch && remote(pick.branch)) from = `origin/${pick.branch}`;
  else if (pick.kind === "pr" && pick.head && !pick.cross_repo) from = `origin/${pick.head}`;
  const fromBase = !from;
  if (fromBase) from = base.trim() || src.base;
  const folder = tilde(folderFor(src, where, b), where.home);
  return { action: "new", from, fromBase, folder, note: t("New branch from {from} · {folder}", { from, folder }) };
}

// prOf is the pull request a pick names: its own number, or the one working on the issue
const prOf = (pick: Pick): number | undefined => (pick.kind === "issue" ? pick.pr : pick.number);

const headPicked = (pick: Pick): boolean => (pick.kind === "pr" || pick.kind === "issue") && !!pick.head && !!prOf(pick);

// forkHead says the branch gets a fork's head, fetched from pull/N/head: any name a fork's pull request is given, an
// issue's when it is that pull request's head or forkBranch of it (worktrees.MakePlan's rule)
function forkHead(pick: Pick, branch: string): boolean {
  if (!headPicked(pick) || !pick.cross_repo) return false;
  return pick.kind === "pr" || branch === pick.head || branch === forkBranch(prOf(pick)!, pick.head!);
}

// fromPR is the note's start when an issue's branch is the head of the pull request working on it
export function fromPR(pick: Pick, branch: string): string {
  const b = branch.trim();
  return pick.kind === "issue" && pick.pr && (pick.head === b || forkHead(pick, b)) ? t("From PR #{n} · ", { n: pick.pr }) : "";
}

// createWords is the primary button's words for the plan
export function createWords(plan: Plan | null, busy: boolean): string {
  if (busy) return t("Working…");
  return plan?.action === "open" ? t("Open worktree") : t("Create worktree");
}
