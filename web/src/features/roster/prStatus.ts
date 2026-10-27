// What a pull request's state says on the page: its checks in a few words, its review in a word, and the tone that
// colours the chip beside the link — from `pane.pr.status`, read with gh on the agent's machine.

import type { PRStatus } from "./api";
import { t } from "@/lib/i18n";

export type Tone = "green" | "red" | "amber" | "dim" | "none";

// checksWords says how the checks stand: failures first, then the ones still running, else how many passed
export function checksWords(s: PRStatus): string {
  const c = s.checks;
  const counted = c.total - c.skipped;
  if (c.fail > 0) return t("{n} failed", { n: c.fail });
  if (c.pending > 0) return t("{n} running", { n: c.pending });
  if (counted <= 0) return "no checks";
  return t("{pass}/{total} checks", { pass: c.pass, total: counted });
}

// reviewWords is the request's standing beyond its checks: merged or closed, a draft, conflicts, the review's verdict
export function reviewWords(s: PRStatus): string {
  if (s.state === "MERGED") return "merged";
  if (s.state === "CLOSED") return "closed";
  if (s.draft) return "draft";
  if (s.mergeable === "CONFLICTING") return "conflicts";
  switch (s.review) {
    case "APPROVED":
      return "approved";
    case "CHANGES_REQUESTED":
      return "changes requested";
    case "REVIEW_REQUIRED":
      return "review pending";
  }
  return "";
}

// checksTone colours the checks chip: dim once merged or closed, red for a failure, amber while they run, green when
// every one passed, none while there are none
export function checksTone(s: PRStatus): Tone {
  if (s.state === "MERGED" || s.state === "CLOSED") return "dim";
  if (s.checks.fail > 0) return "red";
  if (s.checks.pending > 0) return "amber";
  if (s.checks.total - s.checks.skipped > 0 && s.checks.pass > 0) return "green";
  return "none";
}

// reviewTone colours the review chip by what reviewWords says: red for conflicts or changes requested, green for an
// approval, dim for a draft, a merge, a close or a review still pending
export function reviewTone(s: PRStatus): Tone {
  if (s.state === "MERGED" || s.state === "CLOSED" || s.draft) return "dim";
  if (s.mergeable === "CONFLICTING" || s.review === "CHANGES_REQUESTED") return "red";
  if (s.review === "APPROVED") return "green";
  return s.review === "REVIEW_REQUIRED" ? "dim" : "none";
}

const RANK: Record<Tone, number> = { red: 4, amber: 3, green: 2, dim: 1, none: 0 };

// statusTone is the request in one colour, the graver of its two chips: dim once merged or closed, red for anything
// that needs a hand, amber while checks run, green when all is well, none while nothing is known
export function statusTone(s: PRStatus): Tone {
  if (s.state === "MERGED" || s.state === "CLOSED") return "dim";
  const checks = checksTone(s);
  const review = reviewTone(s);
  return RANK[checks] >= RANK[review] ? checks : review;
}

// prWords is the whole state in one line, for a tooltip or a menu row
export function prWords(s: PRStatus): string {
  const review = reviewWords(s);
  return review ? `${checksWords(s)} · ${review}` : checksWords(s);
}
