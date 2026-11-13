// The pull requests on a workspace's card, after munza's better-sidebar: one icon and the number in the state's colour —
// green open, dim draft, violet merged, red closed, failed or in conflict, amber changes requested, a ✓ once it can merge
// — the others its agents named counted beside it, and the words of its tooltip. Pure: CardBadges.tsx draws them.

import { t, tc } from "@/lib/i18n";
import type { PullRequest, Session } from "./api";

export type PRTone = "green" | "dim" | "violet" | "red" | "amber" | "none";
export type PRIcon = "git-pull-request" | "git-merge" | "git-pull-request-closed" | "git-pull-request-draft";

// PRLook is how a request shows: its icon, its colour, its state in words, and whether it can merge now
export type PRLook = { icon: PRIcon; tone: PRTone; label: string; ready: boolean };

// CardPR is one request on the card and the pane whose agent named it
export type CardPR = { paneId: string; pr: PullRequest };

// prLook reads a request's state, a merge or a close winning over what was flagged while it stood open
export function prLook(pr: PullRequest): PRLook {
  const s = pr.status;
  const look = (tone: PRTone, label: string, icon: PRIcon = "git-pull-request", ready = false): PRLook => ({ icon, tone, label, ready });
  if (!s) return look("none", pr.error ? t("state unknown") : t("being read…"));
  if (s.state === "MERGED") return look("violet", t("Merged"), "git-merge");
  if (s.state === "CLOSED") return look("red", t("Closed"), "git-pull-request-closed");
  if (s.draft) return look("dim", t("Draft"), "git-pull-request-draft");
  if (s.mergeable === "CONFLICTING" || s.merge_state === "DIRTY") return look("red", t("Conflicts"));
  if (s.checks.fail > 0) return look("red", t("Checks failed"));
  if (s.review === "CHANGES_REQUESTED") return look("amber", t("Changes requested"));
  if (s.merge_state === "CLEAN" && (s.review === "APPROVED" || !s.review) && !s.threads) return look("green", t("Ready to merge"), "git-pull-request", true);
  return look("green", tc("pr-state", "Open"));
}

// gitlab says whether the request is a GitLab merge request, numbered with a !
const gitlab = (url: string): boolean => url.includes("/-/merge_requests/");

// prNumber is the request's number as its host writes it: #212 on GitHub, !9 on GitLab
export function prNumber(pr: PullRequest): string {
  const m = /\/(?:pull|merge_requests)\/(\d+)/.exec(pr.url) ?? /[#!](\d+)/.exec(pr.label);
  if (!m) return pr.label;
  return `${gitlab(pr.url) ? "!" : "#"}${m[1]}`;
}

// prRepo is the repository the request lives in: owner/repo on GitHub, the project's path on GitLab, else the site
export function prRepo(url: string): string {
  const repo = /^https?:\/\/[^/]+\/([^/]+\/[^/]+)\/pull\/\d+/.exec(url)?.[1] ?? /^https?:\/\/[^/]+\/(.+?)\/-\/merge_requests\/\d+/.exec(url)?.[1];
  return repo ?? /^https?:\/\/([^/]+)/.exec(url)?.[1] ?? url;
}

// prHost is the site a request is opened on, as the card names it
export const prHost = (url: string): string => (gitlab(url) ? "GitLab" : "GitHub");

// prTitle is the request's own title, else its label
export const prTitle = (pr: PullRequest): string => pr.status?.title || pr.label;

// isLinkClick says the click asks for the request's own page: ⌘ on a Mac, Ctrl (or ⌘) elsewhere
export const isLinkClick = (e: { metaKey: boolean; ctrlKey: boolean }, mac: boolean): boolean => (mac ? e.metaKey : e.ctrlKey || e.metaKey);

// prTip is the badge's tooltip: where and how it stands, its title, and how to open it on its site
export function prTip(pr: PullRequest, mac: boolean): string {
  const look = prLook(pr);
  const lines = [`${prRepo(pr.url)}${prNumber(pr)} · ${look.label}`, prTitle(pr)];
  if (!pr.status && pr.error) lines.push(t("Couldn't read it: {reason}", { reason: pr.error }));
  lines.push(mac ? t("⌘-click opens it on {host}", { host: prHost(pr.url) }) : t("Ctrl-click opens it on {host}", { host: prHost(pr.url) }));
  return lines.join("\n");
}

// moreTip lists the requests past the first, a line each
export const moreTip = (prs: CardPR[]): string => prs.map(({ pr }) => `${prRepo(pr.url)}${prNumber(pr)} · ${prTitle(pr)}`).join("\n");

// cardPRs are the requests a workspace's agents named, each once, the one the card speaks for first
export function cardPRs(session: Session, leadPaneId?: string): CardPR[] {
  const all: CardPR[] = [];
  for (const tab of session.tabs) for (const pane of tab.panes) if (pane.agent && pane.pr) all.push({ paneId: pane.id, pr: pane.pr });
  all.sort((a, b) => Number(b.paneId === leadPaneId) - Number(a.paneId === leadPaneId));
  const seen = new Set<string>();
  return all.filter(({ pr }) => !seen.has(pr.url) && seen.add(pr.url));
}
