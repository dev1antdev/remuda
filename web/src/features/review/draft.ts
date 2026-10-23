// The review's draft: comments on diff lines, and the messages they become when sent to the agent.

import type { Review, Verdict } from "./api";

export type DraftComment = {
  id: number;
  file: string;
  line: number; // in the new file; in the old one for a removed line
  removed: boolean;
  code: string; // the diff row with its prefix, part of the comment's address
  text: string;
};

// place names the folder with its branch and HEAD, as the message's address
export const place = (dir: string, review: Review | null) => `${dir}${review?.branch ? ` (${review.branch}${review.head ? ` at ${review.head.slice(0, 7)}` : ""})` : ""}`;

export const COMMIT_ASK = "Commit the reviewed work now, with a message that says what changed for the person, and say when it is committed.";

export const commentLines = (draft: DraftComment[]) =>
  draft.map((comment, index) => `${index + 1}. ${comment.file}:${comment.line}${comment.removed ? " (removed line)" : ""}\n   > ${comment.code}\n   ${comment.text.replace(/\n/g, "\n   ")}`);

// commentsMessage is the draft alone, no verdict: one numbered message
export const commentsMessage = (draft: DraftComment[], where: string) => [`Review comments (${draft.length}) on ${where}:`, ...commentLines(draft)].join("\n\n");

// verdictMessage ends a review: the verdict, the note, the draft, and what it means for the agent; an approval may ask
// for the commit too — the agent commits, remuda never writes into a repository
export function verdictMessage(verdict: Verdict, note: string, draft: DraftComment[], where: string, commit = false): string {
  const parts = [`Review of ${where}: ${verdict === "approved" ? "approved" : "changes requested"}.`];
  if (note.trim()) parts.push(note.trim());
  if (draft.length > 0) parts.push(`Comments (${draft.length}):`, ...commentLines(draft));
  parts.push(
    verdict === "approved"
      ? "The changes reviewed are approved: carry on with your flow, and if this is part of a task, it can move on. Any comments above are notes to take along, not blockers."
      : "Work on what is asked above, then say when the changes are ready for another look. The work is not finished until it is approved.",
  );
  if (verdict === "approved" && commit) parts.push(COMMIT_ASK);
  return parts.join("\n\n");
}
