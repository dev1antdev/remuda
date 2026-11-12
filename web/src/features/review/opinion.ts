// The second opinion on the page: which agent reviews by default, which of its comments sit under which diff row,
// the words of its block in the Changes lens, and what a comment becomes once added to the person's own review.

import { plural } from "@/lib/text";
import { t } from "@/lib/i18n";
import type { DiffRow } from "./hunks";

export type Severity = "high" | "medium" | "low";

// OpinionComment is one finding the reviewer gave, numbered within its opinion
export type OpinionComment = { id: number; path: string; line: number; severity?: Severity; text: string; dismissed?: boolean };

// stopped: a Claude Code reviewer denied what would wait for a permission and stopped on those calls before any answer
export type OpinionState = "running" | "done" | "unreadable" | "stopped" | "gone";

// Opinion is the folder's newest second opinion, as the server keeps it
export type Opinion = {
  machine: string;
  dir: string;
  agent: string;
  model?: string;
  tab: string;
  pane: string;
  asked: string;
  answered?: string;
  state: OpinionState;
  note?: string;
  summary?: string;
  comments: OpinionComment[];
  close: boolean;
  moved?: string[]; // the files of its comments that changed since it was asked: their lines may have moved
};

// reviewerFor is the agent a second opinion asks by default: another vendor than the pane's — Claude Code and Codex
// for each other — else the first other agent herdr knows
export function reviewerFor(paneAgent: string, kinds: string[]): string {
  const has = (kind: string) => kinds.includes(kind);
  if (paneAgent === "claude" && has("codex")) return "codex";
  if (paneAgent !== "claude" && has("claude")) return "claude";
  return kinds.find((kind) => kind !== paneAgent && kind !== "shell") ?? kinds[0] ?? "";
}

// openComments is what is neither added to the person's review nor dismissed
export const openComments = (opinion: Opinion | null | undefined): OpinionComment[] => (opinion ? opinion.comments.filter((c) => !c.dismissed) : []);

// byFile is the open comments by their file, each in the order given
export function byFile(comments: OpinionComment[]): Map<string, OpinionComment[]> {
  const files = new Map<string, OpinionComment[]>();
  for (const comment of comments) files.set(comment.path, [...(files.get(comment.path) ?? []), comment]);
  return files;
}

// asksPerson says whether the reviewer's pane waits on the person: its status, or the radar's reading of it
export const asksPerson = (pane: { status: string; radar?: { state: string } } | undefined): boolean => !!pane && (pane.status === "input" || pane.radar?.state === "blocked");

// onRow says whether a comment sits under the row: the new file's line it names, never a removed line's
export const onRow = (comment: OpinionComment, row: DiffRow): boolean => row.kind !== "meta" && row.kind !== "del" && row.newLine === comment.line;

// offRows is the comments of a file that no row of its diff, as unfolded, carries
export const offRows = (comments: OpinionComment[], rows: DiffRow[]): OpinionComment[] => comments.filter((comment) => !rows.some((row) => onRow(comment, row)));

// unlisted is the open comments on files the lens does not list
export const unlisted = (comments: OpinionComment[], files: string[]): OpinionComment[] => comments.filter((comment) => !files.includes(comment.path));

// Placed is where the open comments go: under the rows of their file's diff, or apart — on files the lens does not
// list, or on files changed since the reviewer was asked, whose lines may no longer be the ones it meant
export type Placed = { onRows: OpinionComment[]; unlisted: OpinionComment[]; moved: OpinionComment[] };

export function placeComments(opinion: Opinion | null | undefined, files: string[]): Placed {
  const open = openComments(opinion);
  const moved = new Set(opinion?.moved ?? []);
  const kept = open.filter((comment) => !moved.has(comment.path));
  return {
    onRows: kept.filter((comment) => files.includes(comment.path)),
    unlisted: unlisted(kept, files),
    moved: open.filter((comment) => moved.has(comment.path)),
  };
}

// whereWords is a comment's place as a note set apart from its row says it: its line, or the file as a whole
export const whereWords = (comment: OpinionComment): string => (comment.line > 0 ? t("line {n}", { n: comment.line }) : t("the whole file"));

// addressOf is a comment's file and line, the file alone for one on the file as a whole
export const addressOf = (comment: OpinionComment): string => (comment.line > 0 ? `${comment.path}:${comment.line}` : comment.path);

// severityWords is a comment's severity as its tag says it
export function severityWords(severity: Severity | undefined): string {
  if (severity === "high") return t("high");
  if (severity === "medium") return t("medium");
  if (severity === "low") return t("low");
  return "";
}

// adoptedText is the comment as it joins the person's draft: its severity, its words, and who said it — in English,
// as everything the draft sends the agent is
export function adoptedText(comment: OpinionComment, reviewer: string): string {
  const severity = comment.severity ? `[${comment.severity}] ` : "";
  return `${severity}${comment.text} — second opinion of ${reviewer}`;
}

// headWords is the block's line: who is reviewing — or waits for the person in its tab (a folder it was never trusted
// in asks first) — or how many comments stand, or why there are none
export function headWords(opinion: Opinion, reviewer: string, asking = false): string {
  const open = openComments(opinion).length;
  switch (opinion.state) {
    case "running":
      if (asking) return t("{agent} is asking you something in its tab", { agent: reviewer });
      return t("{agent} is reviewing — the comments appear here when it answers", { agent: reviewer });
    case "unreadable":
      return t("{agent} answered in a form that could not be read; its answer is in its tab", { agent: reviewer });
    case "stopped":
      return t("{agent} stopped on calls it was denied without asking; they are listed in its tab", { agent: reviewer });
    case "gone":
      return opinion.note || t("the reviewer's tab closed before it answered");
  }
  if (opinion.comments.length === 0) return t("{agent} found nothing to comment on", { agent: reviewer });
  if (open === 0) return t("every comment added or dismissed");
  return plural(open, "comment");
}

// subWords is what the line under the branch adds while an opinion stands: that one is on its way, or how many of its
// comments are open; nothing once they are all dealt with
export function subWords(opinion: Opinion | null): string {
  if (!opinion) return "";
  if (opinion.state === "running") return t("second opinion on its way");
  const open = openComments(opinion).length;
  return open > 0 ? t("second opinion: {comments}", { comments: plural(open, "comment") }) : "";
}
