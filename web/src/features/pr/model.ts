// What GitHub's answers mean for the panel, as plain functions: a pull request's state, its checks one by one and
// together, what stands between it and a merge, the one line of stats every open one shows, what can be done to it,
// and the search the Repo list sends.

import type { IconName } from "@/ui/icons";
import { t, tc, tn } from "@/lib/i18n";
import type { Card, Check, Detail, Problem } from "./api";

// Tone is how a figure or a state is coloured: red, amber, green, the quiet colour, or nothing to say
export type Tone = "bad" | "warn" | "good" | "dim" | "none";

export type Look = { label: string; icon: IconName; tone: Tone | "merged" };

// prLook is a pull request's state as its icon shows it: open green, draft dim, merged in its own colour, closed red
export function prLook(pr: Pick<Card, "state" | "draft">): Look {
  if (pr.state === "MERGED") return { label: t("Merged"), icon: "git-merge", tone: "merged" };
  if (pr.state === "CLOSED") return { label: t("Closed"), icon: "git-pull-request-closed", tone: "bad" };
  if (pr.draft) return { label: t("Draft"), icon: "git-pull-request-draft", tone: "dim" };
  return { label: t("Open"), icon: "git-pull-request", tone: "good" };
}

export type CheckKind = "failure" | "running" | "pending" | "success" | "skipped";

const KIND_ICON: Record<CheckKind, IconName> = { failure: "circle-x", running: "loader-circle", pending: "circle-dot", success: "circle-check", skipped: "circle-slash" };
const KIND_TONE: Record<CheckKind, Tone> = { failure: "bad", running: "warn", pending: "warn", success: "good", skipped: "dim" };

// kindOf is one check, step or job as failed, running, queued, passed or skipped, from its conclusion, else its status;
// a cancelled one counts as failed, as the Wall's chips count it
export function kindOf(check: { conclusion?: string; status?: string }): CheckKind {
  const value = (check.conclusion || check.status || "PENDING").toUpperCase();
  if (["FAILURE", "ERROR", "TIMED_OUT", "ACTION_REQUIRED", "STARTUP_FAILURE", "CANCELLED"].includes(value)) return "failure";
  if (value === "SUCCESS") return "success";
  if (["SKIPPED", "NEUTRAL", "STALE"].includes(value)) return "skipped";
  if (value === "IN_PROGRESS" || value === "RUNNING") return "running";
  return "pending";
}

// kindWords is how a check stands, in words
export function kindWords(kind: CheckKind): string {
  return { failure: tc("check", "Failed"), running: t("In progress"), pending: t("Queued"), success: t("Passed"), skipped: t("Skipped") }[kind];
}

export const kindIcon = (kind: CheckKind): IconName => KIND_ICON[kind];
export const kindTone = (kind: CheckKind): Tone => KIND_TONE[kind];

const WORST_FIRST: CheckKind[] = ["failure", "running", "pending", "success", "skipped"];

export type CheckCounts = Record<CheckKind, number>;

// checkCounts counts the checks by how they stand, and the worst of them
export function checkCounts(checks: { conclusion?: string; status?: string }[]): { counts: CheckCounts; worst: CheckKind | null } {
  const counts: CheckCounts = { failure: 0, running: 0, pending: 0, success: 0, skipped: 0 };
  for (const c of checks) counts[kindOf(c)]++;
  return { counts, worst: checks.length ? (WORST_FIRST.find((k) => counts[k] > 0) ?? "skipped") : null };
}

// cardChecks is a list row's checks: GitHub's rollup of the head commit's, null when it has none
export function cardChecks(card: Pick<Card, "checks">): CheckKind | null {
  return card.checks ? kindOf({ status: card.checks }) : null;
}

export type CheckGroup = { key: string; name: string; run?: number; checks: Check[]; worst: CheckKind };

// checkGroups puts the checks in their workflow run, those that need a look first: a failure, then one still going
export function checkGroups(checks: Check[]): CheckGroup[] {
  const groups = new Map<string, CheckGroup>();
  for (const c of checks) {
    const name = c.workflow || (c.run ? t("GitHub Actions") : t("Other checks"));
    const key = `${name}:${c.run ?? "external"}`;
    const group = groups.get(key) ?? { key, name, run: c.run, checks: [], worst: "skipped" as CheckKind };
    group.checks.push(c);
    groups.set(key, group);
  }
  const rank = (g: CheckGroup) => (g.worst === "failure" ? 0 : g.worst === "running" || g.worst === "pending" ? 1 : 2);
  const out = [...groups.values()].map((g) => ({ ...g, worst: checkCounts(g.checks).worst ?? "skipped", checks: [...g.checks].sort((a, b) => WORST_FIRST.indexOf(kindOf(a)) - WORST_FIRST.indexOf(kindOf(b))) }));
  return out.sort((a, b) => rank(a) - rank(b));
}

export const unresolved = (pr: Pick<Detail, "threads">) => pr.threads.filter((th) => !th.resolved);

export type Section = "overview" | "comments" | "checks" | "files";
export type Blocker = { label: string; tone: Tone; section?: Section };

// blockers is what still stands between an open pull request and a merge, the ones someone has to act on first; GitHub
// merges past whatever it does not require, so a merge's confirm names them
export function blockers(pr: Detail): Blocker[] {
  if (pr.state !== "OPEN") return [];
  const out: Blocker[] = [];
  const { counts } = checkCounts(pr.checks);
  if (pr.draft) out.push({ label: t("Draft"), tone: "warn" });
  if (pr.mergeable === "CONFLICTING") out.push({ label: t("Conflicts"), tone: "bad" });
  if (counts.failure) out.push({ label: tn(counts.failure, "{n} check failed", "{n} checks failed"), tone: "bad", section: "checks" });
  if (pr.review_decision === "CHANGES_REQUESTED") out.push({ label: t("Changes requested"), tone: "bad", section: "overview" });
  const open = unresolved(pr).length;
  if (open) out.push({ label: tn(open, "{n} unresolved thread", "{n} unresolved threads"), tone: "bad", section: "comments" });
  const running = counts.running + counts.pending;
  if (running) out.push({ label: tn(running, "{n} check running", "{n} checks running"), tone: "warn", section: "checks" });
  if (pr.review_decision === "REVIEW_REQUIRED") out.push({ label: t("Review required"), tone: "warn", section: "overview" });
  if (pr.merge_state === "BEHIND") out.push({ label: t("Behind {base}", { base: pr.base }), tone: "warn" });
  if (!out.length && pr.merge_state === "BLOCKED") out.push({ label: t("Blocked by the branch's rules"), tone: "warn" });
  return out;
}

export type Stat = { key: string; label: string; value: string; tone: Tone; detail: string; section?: Section };

// stats is the one line every open pull request shows, always in the same order: checks passed of all, reviews approved
// of those asked, threads unresolved, and the merge's own word only when the others do not say it
export function stats(pr: Detail): Stat[] {
  const { counts } = checkCounts(pr.checks);
  const counted = counts.failure + counts.success + counts.running + counts.pending;
  const running = counts.running + counts.pending;
  const detail = [counts.failure ? t("{n} failed", { n: counts.failure }) : "", running ? t("{n} running", { n: running }) : "", counts.success ? t("{n} passed", { n: counts.success }) : ""].filter(Boolean).join(", ");
  const asked = pr.reviewers.filter((r) => r.state !== "COMMENTED");
  const approved = asked.filter((r) => r.state === "APPROVED").length;
  const open = unresolved(pr).length;
  const decision: Record<string, [Tone, string]> = { CHANGES_REQUESTED: ["bad", t("Changes requested")], REVIEW_REQUIRED: ["warn", t("Review required")], APPROVED: ["good", t("Approved")] };
  const [reviewTone, reviewDetail] = decision[pr.review_decision] ?? ["none", t("No review needed")];
  const out: Stat[] = [
    { key: "checks", label: t("Checks"), value: counted ? `${counts.success}/${counted}` : "–", tone: !counted ? "none" : counts.failure ? "bad" : running ? "warn" : "good", detail: detail || t("No checks reported"), section: "checks" },
    { key: "reviews", label: t("Reviews"), value: asked.length ? `${approved}/${asked.length}` : "–", tone: reviewTone, detail: reviewDetail, section: "overview" },
    { key: "unresolved", label: t("Unresolved"), value: String(open), tone: open ? "bad" : "none", detail: open ? tn(open, "{n} unresolved thread", "{n} unresolved threads") : t("No unresolved threads"), section: "comments" },
  ];
  const merge = mergeStat(pr);
  return merge ? [...out, merge] : out;
}

function mergeStat(pr: Detail): Stat | null {
  const stat = (value: string, tone: Tone, detail: string): Stat => ({ key: "merge", label: t("Merge"), value, tone, detail });
  if (pr.auto_merge) return stat(t("Auto"), "warn", t("Merging by itself once it is ready"));
  if (pr.draft) return stat(t("Draft"), "warn", t("Still a draft"));
  if (pr.mergeable === "CONFLICTING") return stat(t("Conflicts"), "bad", t("Conflicts with {base}", { base: pr.base }));
  if (pr.merge_state === "BEHIND") return stat(t("Behind"), "warn", t("Behind {base}", { base: pr.base }));
  return blockers(pr).length ? null : stat(t("Ready"), "good", t("Ready to merge"));
}

export type Action = "merge" | "squash" | "rebase" | "auto" | "cancel-auto" | "close" | "reopen";

// actions is what can be done to the pull request, the likeliest first; a merged one has none. Merge when ready is
// offered only where GitHub has something required to wait for (BLOCKED or BEHIND): elsewhere it would merge at once
export function actions(pr: Detail): Action[] {
  if (pr.state === "MERGED") return [];
  if (pr.state === "CLOSED") return ["reopen"];
  if (pr.draft) return ["close"];
  if (pr.auto_merge) return ["cancel-auto", "close"];
  const methods = (pr.merge_methods.length ? pr.merge_methods : ["merge"]).filter((m): m is Action => m === "merge" || m === "squash" || m === "rebase");
  const waits = pr.auto_allowed && (pr.merge_state === "BLOCKED" || pr.merge_state === "BEHIND") ? (["auto"] as Action[]) : [];
  return [...waits, ...methods, "close"];
}

export const merges = (a: Action) => a === "merge" || a === "squash" || a === "rebase";

// actionLabel is an action's row in the menu and its confirm's button
export function actionLabel(a: Action): string {
  return {
    merge: t("Merge"),
    squash: t("Squash and merge"),
    rebase: t("Rebase and merge"),
    auto: t("Merge when ready"),
    "cancel-auto": t("Cancel merge when ready"),
    close: t("Close"),
    reopen: t("Reopen"),
  }[a];
}

// actionQuestion is what the confirm card asks
export function actionQuestion(a: Action, pr: Pick<Detail, "number" | "head" | "base">): string {
  const vars = { n: pr.number, head: pr.head, base: pr.base };
  switch (a) {
    case "merge":
      return t("Merge #{n} from {head} into {base}? This cannot be undone.", vars);
    case "squash":
      return t("Squash and merge #{n} from {head} into {base}? This cannot be undone.", vars);
    case "rebase":
      return t("Rebase and merge #{n} from {head} into {base}? This cannot be undone.", vars);
    case "auto":
      return t("Merge #{n} by itself once its checks and reviews pass? Anything pushed before then goes in too.", vars);
    case "cancel-auto":
      return t("Stop #{n} from merging by itself?", vars);
    case "close":
      return t("Close #{n} without merging?", vars);
    case "reopen":
      return t("Reopen #{n}?", vars);
  }
}

// actionDone is the toast once GitHub took it
export function actionDone(a: Action): string {
  return { merge: t("Merged."), squash: t("Squashed and merged."), rebase: t("Rebased and merged."), auto: t("It will merge once it is ready."), "cancel-auto": t("It will not merge by itself."), close: t("Closed."), reopen: t("Reopened.") }[a];
}

// squashMessage is the commit message a squash starts with: the title and number, a blank line, the description
export function squashMessage(pr: Pick<Detail, "title" | "number" | "body">): string {
  const subject = `${pr.title} (#${pr.number})`;
  return pr.body.trim() ? `${subject}\n\n${pr.body.trim()}` : subject;
}

// splitMessage is a commit message's subject (its first line) and body, as git reads one
export function splitMessage(text: string): { subject: string; body: string } {
  const trimmed = text.trim();
  const nl = trimmed.indexOf("\n");
  return nl < 0 ? { subject: trimmed, body: "" } : { subject: trimmed.slice(0, nl).trim(), body: trimmed.slice(nl + 1).trim() };
}

// problemWords is a gh that cannot be used on the machine, in one sentence
export function problemWords(p: Problem): string {
  return p.kind === "missing" ? t("The GitHub CLI is not installed on {machine}", { machine: p.machine }) : t("The GitHub CLI is not signed in on {machine}", { machine: p.machine });
}

// searchText is what the Repo list sends: three characters at least, or a number, any length; "" sends nothing new
export function searchText(text: string): string {
  const q = text.trim();
  return q.length >= 3 || /^#?[1-9][0-9]*$/.test(q) ? q : "";
}
