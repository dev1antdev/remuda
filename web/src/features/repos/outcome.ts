// What a fetch, pull or push came to, in the page's words: a toast's line, an error when it is one. Any screen that
// offers the three (the Git lens too) says them with these.

import { VENDOR_NAME } from "@/ui/brand";
import { t, tn } from "@/lib/i18n";
import type { Outcome, SyncAction } from "./syncApi";

export type Said = { text: string; error: boolean };

const agentName = (kind: string | undefined) => (kind && VENDOR_NAME[kind]) || t("An agent");

// busyWords is why a pull waits for an agent in the checkout
function busyWords(o: Outcome, name: string): string {
  const agent = agentName(o.agent);
  if (o.doing === "asking") return t("{agent} is asking you something in {name}: a pull now would change files under it. Answer it first.", { agent, name });
  if (o.doing === "ending") return t("{agent}'s turn in {name} is still ending: pull again in a moment.", { agent, name });
  return t("{agent} is at work in {name}: a pull now would change files under it. Pull once it has finished.", { agent, name });
}

// refusedWords is why remuda did not run it
function refusedWords(o: Outcome, name: string): string {
  switch (o.reason) {
    case "away":
      return o.output || t("The machine cannot be reached now.");
    case "missing":
      return t("{name} is no longer there.", { name });
    case "not-repository":
      return t("{name} is no git checkout.", { name });
    case "running":
      return t("Something already runs in {name}: wait for it to end.", { name });
    case "detached":
      return t("{name} is on no branch (a detached HEAD): nothing to pull or push.", { name });
    case "no-remote":
      return t("{name} has no remote.", { name });
    case "which-remote":
      return t("{branch} has no upstream, and {name} has several remotes but no origin: push it once from a terminal to choose.", { branch: o.branch ?? "", name });
    case "no-upstream":
      return t("{branch} tracks no branch of a remote: nothing to pull. Push sets one.", { branch: o.branch ?? "" });
    case "local-upstream":
      return t("{branch} tracks a local branch, no remote.", { branch: o.branch ?? "" });
    case "gone":
      return t("{name}: its upstream is no longer on the remote.", { name });
    case "diverged":
      return t("{name} has diverged: {ahead} of its own, {behind} not pulled. A fast-forward cannot join them, so nothing changed: merge or rebase it.", {
        name,
        ahead: tn(o.ahead ?? 0, "{n} commit", "{n} commits"),
        behind: tn(o.behind ?? 0, "{n} commit", "{n} commits"),
      });
    case "busy":
      return busyWords(o, name);
  }
  return t("{name}: nothing done.", { name });
}

function failedWords(action: SyncAction, name: string, why: string): string {
  if (action === "fetch") return t("The fetch of {name} failed: {why}", { name, why });
  if (action === "pull") return t("The pull into {name} failed: {why}", { name, why });
  return t("The push of {name} failed: {why}", { name, why });
}

// outcomeWords is what the action came to, named by its repository
export function outcomeWords(o: Outcome, name: string): Said {
  if (o.state === "failed") return { text: failedWords(o.action, name, o.output || t("git gave no reason")), error: true };
  if (o.state === "refused") return { text: refusedWords(o, name), error: true };
  if (o.state === "held") return { text: t("The push of {name} waits for the secret check.", { name }), error: true };
  if (o.action === "fetch") {
    const behind = o.repo?.behind ?? 0;
    return { text: behind ? t("Fetched {name}: {pulled}", { name, pulled: tn(behind, "{n} not pulled", "{n} not pulled") }) : t("Fetched {name}", { name }), error: false };
  }
  if (o.action === "pull") {
    if (o.reason === "up-to-date") return { text: t("{name} is up to date", { name }), error: false };
    return { text: t("Pulled {commits} into {name}", { commits: tn(o.count ?? 0, "{n} commit", "{n} commits"), name }), error: false };
  }
  if (o.reason === "new-branch") return { text: t("Pushed {branch} to {remote}, its upstream from now on", { branch: o.branch ?? "", remote: o.remote ?? "" }), error: false };
  if (!o.count) return { text: t("Pushed {branch} to {remote}", { branch: o.branch ?? "", remote: o.remote ?? "" }), error: false };
  return { text: t("Pushed {commits} of {branch} to {remote}", { commits: tn(o.count, "{n} commit", "{n} commits"), branch: o.branch ?? "", remote: o.remote ?? "" }), error: false };
}

// FetchRun is Fetch all as it goes: how many of how many, and the repositories that failed with git's line
export type FetchRun = { done: number; total: number; failed: { name: string; why: string }[] };

// runWords is Fetch all's button while it runs
export const runWords = (run: FetchRun): string => t("Fetching {done}/{total}…", { done: run.done, total: run.total });

// ranWords is the toast once Fetch all ended: what was fetched, and each failure by its repository and git's line
export function ranWords(run: FetchRun): Said {
  const repositories = (n: number) => tn(n, "{n} repository", "{n} repositories");
  if (!run.failed.length) return { text: t("Fetched {repositories}", { repositories: repositories(run.total) }), error: false };
  const named = run.failed
    .slice(0, 3)
    .map((f) => `${f.name}: ${f.why.split("\n").pop()}`)
    .join("; ");
  const rest = run.failed.length > 3 ? ` ${t("and {n} more", { n: run.failed.length - 3 })}` : "";
  return { text: `${t("Fetched {done} of {repositories}.", { done: run.total - run.failed.length, repositories: repositories(run.total) })} ${named}${rest}`, error: true };
}

// heldWords is the held push's head: how many findings, or that the check could not clear it
export function heldWords(o: Outcome, name: string): string {
  if (o.reason === "check-failed") return t("The secret check could not read what {name} would push: {why}. Nothing is known about it.", { name, why: o.output || t("no reason given") });
  const found = (o.scan?.findings.length ?? 0) + (o.scan?.more ?? 0);
  return tn(found, "The secret check found {n} thing in what {name} would push, or in its work beside it, that looks like a secret.", "The secret check found {n} things in what {name} would push, or in its work beside it, that look like secrets.", { name });
}

// pushAnywayAsk is the confirm before a push the check held back
export function pushAnywayAsk(o: Outcome, name: string): string {
  if (o.reason === "check-failed") return t("Push {name} with no secret check? Nothing is known about what it carries.", { name });
  const found = (o.scan?.findings.length ?? 0) + (o.scan?.more ?? 0);
  return tn(found, "Push {name} with {n} finding? Once on the remote a secret has to be revoked, not just deleted.", "Push {name} with {n} findings? Once on the remote a secret has to be revoked, not just deleted.", { name });
}
