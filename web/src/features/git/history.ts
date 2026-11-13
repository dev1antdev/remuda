// The history the lens draws: the pages read so far, laid out on lanes as one fold, the uncommitted changes as a row of
// their own above HEAD when the tree is dirty. A page more goes on from the fold's state; only the uncommitted row
// coming or going, or the refs moving, lays it out again.

import type { Commit, GitLog } from "./api";
import { foldLanes, type LaneRow, type LaneState } from "./lanes";

export const PAGE = 300;

export type History = { log: GitLog; commits: Commit[]; rows: LaneRow[]; lanes: LaneState; uncommitted: boolean };

// the row the uncommitted changes take: dashed down to HEAD, never a commit of its own
const pending = (head: string | undefined) => ({ hash: "*", parents: head ? [head] : [], dashed: true });

// historyOf lays a first read out, the uncommitted row on top when asked
export function historyOf(log: GitLog, uncommitted: boolean): History {
  const shown = uncommitted && !!log.head;
  const { rows, state } = foldLanes([...(shown ? [pending(log.head)] : []), ...log.commits]);
  return { log, commits: log.commits, rows, lanes: state, uncommitted: shown };
}

// withPage adds the next page; null when the refs moved since the first one, so the history is read again from the top
export function withPage(history: History, page: GitLog): History | null {
  if (page.fingerprint !== history.log.fingerprint || page.skip !== history.commits.length) return null;
  const { rows, state } = foldLanes(page.commits, history.lanes);
  return { log: { ...history.log, more: page.more }, commits: [...history.commits, ...page.commits], rows: [...history.rows, ...rows], lanes: state, uncommitted: history.uncommitted };
}

// withUncommitted keeps the history when the uncommitted row stands as it should, else lays it out again with or without it
export function withUncommitted(history: History, dirty: boolean): History {
  const want = dirty && !!history.log.head;
  if (want === history.uncommitted) return history;
  const { rows, state } = foldLanes([...(want ? [pending(history.log.head)] : []), ...history.commits]);
  return { ...history, rows, lanes: state, uncommitted: want };
}

// rowCommit is the commit a row draws, null for the uncommitted row
export function rowCommit(history: History, index: number): Commit | null {
  const at = history.uncommitted ? index - 1 : index;
  return at >= 0 ? (history.commits[at] ?? null) : null;
}
