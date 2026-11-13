// How the issues panel says things: an issue's state as GitHub draws it (open green, closed as completed purple, not
// planned grey), a comment's role badges, a reaction's emoji (GitHub's content, as a label's text is), a reference.

import type { IconName } from "@/ui/icons";
import { t } from "@/lib/i18n";
import type { Here, Item, Person } from "./api";

export type Tone = "open" | "done" | "skipped";
export type StateLook = { label: string; tone: Tone; icon: IconName; title: string };

export function stateLook(issue: Pick<Item, "state" | "reason">): StateLook {
  if (issue.state === "open") return { label: t("Open"), tone: "open", icon: "circle-dot", title: t("Open") };
  if (issue.reason === "not_planned") return { label: t("Closed"), tone: "skipped", icon: "circle-slash", title: t("Closed as not planned") };
  return { label: t("Closed"), tone: "done", icon: "circle-check", title: t("Closed as completed") };
}

const ROLES: Record<string, string> = { OWNER: "Owner", MEMBER: "Member", COLLABORATOR: "Collaborator", CONTRIBUTOR: "Contributor" };

// roleBadges are a comment's badges: Author when the issue's author wrote it, then the writer's tie to the repository
export function roleBadges(writer: string, author: string, role?: string): string[] {
  const out: string[] = [];
  if (writer === author) out.push(t("Author"));
  const tie = role ? ROLES[role] : undefined;
  if (tie) out.push(t(tie));
  return out;
}

export const REACTIONS: Record<string, string> = { "+1": "👍", "-1": "👎", laugh: "😄", hooray: "🎉", confused: "😕", heart: "❤️", rocket: "🚀", eyes: "👀" };

// refWords is how another issue is named: #12 in this repository, owner/repo#12 in another
export function refWords(repo: string, other: string, number: number): string {
  return !other || other.toLowerCase() === repo.toLowerCase() ? `#${number}` : `${other}#${number}`;
}

// shortSha is a commit as GitHub shows it, its first seven hex digits
export const shortSha = (sha: string) => sha.slice(0, 7);

// issueClosedWords is the toast after a close: which issue, and for what
export function issueClosedWords(repo: string, number: number, reason: "completed" | "not_planned"): string {
  const ref = `${repo}#${number}`;
  return reason === "not_planned" ? t("Closed {ref} as not planned", { ref }) : t("Closed {ref} as completed", { ref });
}

export function assigned(issue: Pick<Item, "assignees">, me: string): boolean {
  return !!me && issue.assignees.some((a) => a.login.toLowerCase() === me.toLowerCase());
}

// initials is what stands in for a person's face: the first letter of the login
export const initials = (person: Person) => (person.login.replace(/[^a-z0-9]/gi, "")[0] ?? "?").toUpperCase();

// hueOf gives a person a steady one of six accents, by their login
export function hueOf(login: string): number {
  let h = 7;
  for (const ch of login) h = (h * 31 + ch.charCodeAt(0)) >>> 0;
  return h % 6;
}

// whyNoRepo says why the folder is no GitHub checkout, with what would make it one
export function whyNoRepo(here: Here | undefined, machine: string): { text: string; run?: string } {
  switch (here?.why) {
    case "no-folder":
      return { text: t("The folder is not there on {machine} any more.", { machine }) };
    case "no-git":
      return { text: t("The folder is not a git checkout, so it has no GitHub repository.") };
    case "no-origin":
      return { text: t("This checkout has no origin: it is on GitHub once one names the repository."), run: "git remote add origin https://github.com/OWNER/REPO.git" };
    case "not-github":
      return { text: t("This checkout's origin is on {host}, not GitHub: these issues are GitHub's.", { host: here.host ?? "" }) };
  }
  return { text: t("Pick a repository: no workspace on {machine} is a checkout of a GitHub repository.", { machine }) };
}
