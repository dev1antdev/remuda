// What a workspace's hover card says, after Orca's: the note, the pull requests, the issues, the live ports and how
// fresh the requests' state is. Pure: CardHover.tsx draws it.

import { issueTone } from "@/features/jira/jira";
import { t, tn } from "@/lib/i18n";
import { ago } from "@/lib/time";
import type { JiraIssue, LivePort, Session } from "./api";
import { cardPRs, prHost, type CardPR } from "./cardPR";

// NOTE_MAX is the most a note holds, in bytes, as the server keeps it
export const NOTE_MAX = 4096;

// noteDir is the folder a workspace's note belongs to: its own checkout
export const noteDir = (s: Session): string => s.dir || s.worktree?.checkout_path || "";

// noteBytes is a note's size as the server counts it
export const noteBytes = (text: string): number => new TextEncoder().encode(text.trim()).length;

// NOTE_TIP is the longest stretch of a note its icon's tooltip shows
const NOTE_TIP = 600;

// noteTip is what the note icon's tooltip says: the note, cut at a word past NOTE_TIP characters
export function noteTip(text: string): string {
  if (text.length <= NOTE_TIP) return text;
  const cut = text.slice(0, NOTE_TIP);
  const space = cut.lastIndexOf(" ");
  return `${space > NOTE_TIP / 2 ? cut.slice(0, space) : cut}…`;
}

// portsWords is the plug's tooltip: how many ports and which
export const portsWords = (ports: LivePort[]): string =>
  tn(ports.length, "{n} live port: {ports}", "{n} live ports: {ports}", { ports: ports.map((p) => p.port).join(", ") });

// CardIssue is an issue the card names: a Jira issue its work names, or a GitHub issue its pull request closes
export type CardIssue = { key: string; title: string; where: string; state: string; tone: string; url: string; jira: boolean };

// cardIssues are the Jira issues the workspace names and the GitHub issues its requests close, each once
export function cardIssues(jira: JiraIssue[] | undefined, prs: CardPR[]): CardIssue[] {
  const out: CardIssue[] = (jira ?? []).map((issue) => ({ key: issue.key, title: issue.summary || issue.key, where: "Jira", state: issue.status, tone: issueTone(issue.category), url: issue.url, jira: true }));
  const seen = new Set(out.map((i) => i.url));
  for (const { pr } of prs)
    for (const issue of pr.status?.closes ?? []) {
      if (seen.has(issue.url)) continue;
      seen.add(issue.url);
      out.push({ key: `#${issue.number}`, title: t("Closed by {pr} once it merges", { pr: pr.label }), where: issue.repo, state: "", tone: "", url: issue.url, jira: false });
    }
  return out;
}

// Details are what a workspace's hover card holds; any says whether there is anything to open it for
export type Details = { note: string; prs: CardPR[]; issues: CardIssue[]; ports: LivePort[]; any: boolean };

// detailsOf is a workspace's card details, the request of the pane its card speaks for first; a folder the
// screen-sharing mode hides has none
export function detailsOf(session: Session, leadPaneId: string | undefined, hidden: boolean): Details {
  if (hidden) return { note: "", prs: [], issues: [], ports: [], any: false };
  const prs = cardPRs(session, leadPaneId);
  const issues = cardIssues(session.jira, prs);
  const note = session.note ?? "";
  const ports = session.ports ?? [];
  return { note, prs, issues, ports, any: !!note || prs.length > 0 || issues.length > 0 || ports.length > 0 };
}

// Freshness is how the card's requests stand with their site: being read, the last read failed, or when it answered
export type Freshness = { kind: "reading" | "failed" | "updated" | "none"; host: string; at?: string; reason?: string };

// freshnessOf reads the requests' reads: any under way, else any failed, else when the site answered — each time the
// oldest status shown, so the words never say the card is fresher than all it shows
export function freshnessOf(prs: CardPR[]): Freshness {
  const host = prs[0] ? prHost(prs[0].pr.url) : "GitHub";
  if (!prs.length) return { kind: "none", host };
  if (prs.some(({ pr }) => pr.reading)) return { kind: "reading", host };
  const oldest = prs.map(({ pr }) => pr.status?.at).filter((at): at is string => !!at).sort()[0];
  const failed = prs.find(({ pr }) => pr.error);
  if (failed) return { kind: "failed", host, at: oldest, reason: failed.pr.error };
  if (oldest) return { kind: "updated", host, at: oldest };
  return { kind: "none", host };
}

// freshWords is the card's foot line: "Refreshing from GitHub…", "Couldn't reach GitHub · from 5 min ago", "Updated 3 min ago"
export function freshWords(f: Freshness, now: number): string {
  switch (f.kind) {
    case "reading":
      return t("Refreshing from {host}…", { host: f.host });
    case "failed":
      return f.at && now - new Date(f.at).getTime() >= 60_000 ? t("Couldn't reach {host} · from {ago}", { host: f.host, ago: ago(f.at, now) }) : t("Couldn't reach {host}", { host: f.host });
    case "updated":
      return f.at ? t("Updated {ago}", { ago: ago(f.at, now) }) : "";
  }
  return "";
}
