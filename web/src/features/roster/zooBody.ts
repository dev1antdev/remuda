// What a zoo creature's body says beyond its pose, all of it off the roster: its girth is how full its context is (a
// belt from 80 %, slim again after a compaction), a pile of coins what its conversation cost, a flag how its pull
// request's checks stand (a crown once merged), a mug of coffee and then sweat for a long stretch of work, a battery
// for a plan limit, and the babies around it its subagents at work. The pure part; app/zoo draws them.

import { resetForHit } from "@/features/usage/limits";
import { t, tn } from "@/lib/i18n";
import type { LimitWindow, PaneLimit, PaneUsage, PullRequest } from "./api";
import type { Display } from "./radar";

// girthOf is how full the context is, 0 to 1: the share of the model's window the last response read
export function girthOf(usage: Pick<PaneUsage, "context" | "window"> | undefined): number {
  if (!usage || !(usage.window > 0) || !(usage.context > 0)) return 0;
  return Math.min(1, usage.context / usage.window);
}

// BELT_AT is the girth from which the creature wears its belt: the meter's amber starts at 70 %, the compaction notice at 80
export const BELT_AT = 0.8;

// compacted says whether a context shrank the way a compaction (or a clear) shrinks it: from a third of the window or
// more to under half of what it was
export function compacted(before: Pick<PaneUsage, "context" | "window"> | undefined, after: Pick<PaneUsage, "context" | "window"> | undefined): boolean {
  if (!before || !after || !(before.window > 0)) return false;
  return before.context >= before.window / 3 && after.context < before.context / 2;
}

// COIN_AT are the costs at which the pile gains a coin: a few cents, a quarter, a dollar, three, ten, thirty
export const COIN_AT = [0.05, 0.25, 1, 3, 10, 30];

// coinsOf is how many coins a conversation's cost piles up, 0 to 6
export const coinsOf = (cost: number | undefined): number => COIN_AT.filter((at) => (cost ?? 0) >= at).length;

export type Flag = "" | "open" | "draft" | "pending" | "pass" | "fail" | "merged";

// flagOf is the flag a pull request flies: merged (a crown instead), failed checks, running ones, all passed, a draft, an
// open request with no checks; none without a request, one closed unmerged, or one whose state gh could not read
export function flagOf(pr: PullRequest | undefined): Flag {
  const status = pr?.status;
  if (!status) return "";
  if (status.state === "MERGED") return "merged";
  if (status.state !== "OPEN") return "";
  if (status.checks.fail > 0) return "fail";
  if (status.checks.pending > 0) return "pending";
  if (status.draft) return "draft";
  if (status.checks.total > 0) return "pass";
  return "open";
}

// COFFEE_AFTER and SWEAT_AFTER are how long a stretch of work runs before the mug comes out and before the sweat does
export const COFFEE_AFTER = 20 * 60_000;
export const SWEAT_AFTER = 60 * 60_000;

// workExtras are the mug and the sweat: a long stretch of work, or sweat for anyone in a hot pen
export function workExtras(display: Display, since: number | undefined, now: number, hot = false): { coffee: boolean; sweat: boolean } {
  const working = display === "working";
  const lasted = working && since ? now - since : 0;
  return { coffee: lasted >= COFFEE_AFTER, sweat: hot || lasted >= SWEAT_AFTER };
}

// drainedUntil is when an agent stopped on its plan limit carries on: the person's booked continue, else the reset its
// windows wait on
export function drainedUntil(limit: PaneLimit | undefined, windows: LimitWindow[], agent: string): string | undefined {
  if (!limit?.hit_at) return undefined;
  return limit.continue_at || resetForHit(windows, agent);
}

export type Brood = { working?: number; quiet?: number; done?: number };

// MAX_BABIES is how many babies stand by a creature; the rest are counted
export const MAX_BABIES = 3;

// babiesOf is how many of the conversation's subagents stand by the creature, at work, and how many more there are; a
// quiet one (silent for long without a finish, most likely gone) is no baby, it would stand there for good
export function babiesOf(brood: Brood | undefined): { babies: number; more: number } {
  const working = Math.max(0, brood?.working ?? 0);
  return { babies: Math.min(working, MAX_BABIES), more: Math.max(0, working - MAX_BABIES) };
}

// flagWords is the flag in words, for the creature's tooltip
export function flagWords(flag: Flag, label: string): string {
  switch (flag) {
    case "merged":
      return t("{pr} merged", { pr: label });
    case "fail":
      return t("{pr}: checks failed", { pr: label });
    case "pending":
      return t("{pr}: checks running", { pr: label });
    case "pass":
      return t("{pr}: checks passed", { pr: label });
    case "draft":
      return t("{pr}: a draft", { pr: label });
    case "open":
      return label;
    default:
      return "";
  }
}

// broodWords is the babies in words: "2 subagents working"
export const broodWords = (brood: Brood | undefined): string => (brood?.working ? tn(brood.working, "{n} subagent working", "{n} subagents working") : "");
