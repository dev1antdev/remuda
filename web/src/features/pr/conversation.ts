// The Comments tab's pure part: everything said on the pull request in time order — comments, reviews with something to
// say, threads on lines — narrowed by author and to the unresolved threads; which comments this device has not seen
// (a fingerprint of each one's id and text, so an edit reads as new); the end of a thread's hunk.

import { t } from "@/lib/i18n";
import type { Comment, Detail, Thread } from "./api";

export type Entry = { id: string; kind: string; at: string; comment?: Comment; thread?: Thread };

// reviewWords is a review's verdict as an entry's head says it
function reviewWords(state?: string): string {
  return { APPROVED: t("Approved"), CHANGES_REQUESTED: t("Changes requested"), COMMENTED: t("Reviewed"), DISMISSED: t("Dismissed") }[state ?? ""] ?? t("Review");
}

// entries is the conversation in order: a review shows only when it says something or decides, a thread by its first
// comment's time; with author, the entries that person spoke in; with open, the unresolved threads alone
export function entries(pr: Pick<Detail, "comments" | "reviews" | "threads">, filter: { author?: string; open?: boolean } = {}): Entry[] {
  const all: Entry[] = [
    ...pr.comments.map((c) => ({ id: c.id, kind: t("Comment"), at: c.at, comment: c })),
    ...pr.reviews.filter((r) => r.body.trim() || r.state !== "COMMENTED").map((r) => ({ id: r.id, kind: reviewWords(r.state), at: r.at, comment: r })),
    ...pr.threads.map((th) => ({ id: th.id, kind: th.resolved ? t("Resolved") : t("Unresolved"), at: th.comments[0]?.at ?? "", thread: th })),
  ];
  return all
    .filter((e) => (!filter.open || (e.thread && !e.thread.resolved)) && (!filter.author || (e.thread ? e.thread.comments.some((c) => c.author === filter.author) : e.comment?.author === filter.author)))
    .sort((a, b) => a.at.localeCompare(b.at));
}

// commentsOf is an entry's comments: a thread's in order, else the one
export const commentsOf = (e: Entry): Comment[] => (e.thread ? e.thread.comments : e.comment ? [e.comment] : []);

// authorsOf is everyone who said something, in name order
export function authorsOf(pr: Pick<Detail, "comments" | "reviews" | "threads">): string[] {
  const names = new Set<string>();
  for (const e of entries(pr)) for (const c of commentsOf(e)) names.add(c.author);
  return [...names].sort((a, b) => a.localeCompare(b));
}

// fingerprint is a comment as this device last saw it: its id and a hash of its text, so an edit reads as new
export function fingerprint(c: Pick<Comment, "id" | "body">): string {
  let h = 2166136261;
  for (let i = 0; i < c.body.length; i++) h = Math.imul(h ^ c.body.charCodeAt(i), 16777619);
  return `${c.id}:${(h >>> 0).toString(36)}`;
}

// MAX_SEEN is how many fingerprints a device keeps, the newest
export const MAX_SEEN = 2000;

// unseen is the ids of the comments this device has not seen as they read now
export function unseen(pr: Pick<Detail, "comments" | "reviews" | "threads">, seen: string[]): Set<string> {
  const known = new Set(seen);
  const out = new Set<string>();
  for (const e of entries(pr)) for (const c of commentsOf(e)) if (!known.has(fingerprint(c))) out.add(c.id);
  return out;
}

// markSeen adds the pull request's comments as they read now to what was seen, the oldest falling off past MAX_SEEN
export function markSeen(pr: Pick<Detail, "comments" | "reviews" | "threads">, seen: string[]): string[] {
  const now = entries(pr).flatMap((e) => commentsOf(e).map(fingerprint));
  const kept = seen.filter((f) => !now.includes(f));
  return [...kept, ...now].slice(-MAX_SEEN);
}

// HUNK_LINES is how much of a thread's hunk shows: a comment is about the hunk's end, which can run to hundreds of lines
export const HUNK_LINES = 8;

// hunkEnd is the hunk's @@ line and its last HUNK_LINES lines
export function hunkEnd(hunk: string): string {
  const lines = hunk.replace(/\n+$/, "").split("\n");
  if (lines.length <= HUNK_LINES + 1) return lines.join("\n");
  return [lines[0]!, ...lines.slice(-HUNK_LINES)].join("\n");
}

// whereWords is a thread's place: path:line
export const whereWords = (th: Pick<Thread, "path" | "line">) => (th.line ? `${th.path}:${th.line}` : th.path);
