// Which prefs follow a person from device to device, and how two copies of one are reconciled: the newer stamp wins.
// To let another pref travel, add its key (or its prefix with a trailing dot) to SHARED — the rest of the sync is generic.

export const SHARED = ["theme", "theme.dark", "theme.accent", "radar", "wall", "review.split", "chat.timeline", "footprint", "tasks.view", "standup.period", "code.follow", "panel."] as const;

// isShared says whether a pref key travels: an exact name, or anything under a prefix that ends in a dot
export function isShared(key: string): boolean {
  return SHARED.some((shared) => (shared.endsWith(".") ? key.startsWith(shared) && key.length > shared.length : key === shared));
}

// Stamped is a pref's value and when it was set; an empty stamp means "set before stamps existed", the oldest of all
export type Stamped = { value: unknown; at: string };

const time = (at: string): number => (at ? Date.parse(at) || 0 : 0);

// merge picks the copy to keep: the newer stamp, the remote one when they are equal
export function merge(local: Stamped, remote: Stamped): Stamped {
  return time(local.at) > time(remote.at) ? local : remote;
}

// Plan says what a sync does: apply these remote copies here, push these local ones to the server
export type Plan = { apply: Record<string, Stamped>; push: Record<string, Stamped> };

// plan reconciles the shared keys of both sides: a key only one side has goes to the other (a stampless local one gets a stamp
// of now on its way up), a key both have follows merge; a key that is not shared is ignored wherever it came from
export function plan(local: Record<string, Stamped>, remote: Record<string, Stamped>, now: string): Plan {
  const out: Plan = { apply: {}, push: {} };
  for (const key of new Set([...Object.keys(local), ...Object.keys(remote)])) {
    if (!isShared(key)) continue;
    const mine = local[key];
    const theirs = remote[key];
    if (!theirs && mine) {
      out.push[key] = { value: mine.value, at: mine.at || now };
    } else if (theirs && !mine) {
      out.apply[key] = theirs;
    } else if (mine && theirs) {
      const winner = merge(mine, theirs);
      if (winner === mine && time(mine.at) !== time(theirs.at)) out.push[key] = mine;
      else if (winner === theirs && (time(mine.at) !== time(theirs.at) || JSON.stringify(mine.value) !== JSON.stringify(theirs.value))) out.apply[key] = theirs;
    }
  }
  return out;
}
