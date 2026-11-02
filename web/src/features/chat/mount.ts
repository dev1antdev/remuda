// A long conversation mounts progressively: the last FIRST_TURNS at once, the earlier ones prepended MOUNT_STEP at a time
// in the browser's idle moments, so the first paint costs a screenful and not the whole file. The counts are pure; idle()
// is the one call into the browser.

export const FIRST_TURNS = 200;
export const MOUNT_STEP = 120;

// ALL_REVEALED is the revealed count once every turn is mounted: it stays so as turns arrive, else each new turn at the
// end would push the first one out of the log until the next idle slice put it back
export const ALL_REVEALED = Number.POSITIVE_INFINITY;

// hiddenCount is how many turns from the start still wait: all but the last `first`, less what was revealed since
export function hiddenCount(total: number, revealed: number, first = FIRST_TURNS): number {
  return Math.max(0, total - first - revealed);
}

// nextReveal is the revealed count after one more slice; the slice that reaches the start reveals everything for good
export function nextReveal(total: number, revealed: number, step = MOUNT_STEP, first = FIRST_TURNS): number {
  const next = revealed + step;
  return next >= total - first ? ALL_REVEALED : next;
}

// idle runs fn when the window has a moment, within a third of a second at the latest; the return cancels it
export function idle(win: Window, fn: () => void): () => void {
  if (typeof win.requestIdleCallback === "function") {
    const id = win.requestIdleCallback(fn, { timeout: 300 });
    return () => win.cancelIdleCallback(id);
  }
  const id = win.setTimeout(fn, 50);
  return () => win.clearTimeout(id);
}

// anchorsItself says whether the browser keeps the view still when content is added above it (scroll anchoring); where it
// does not, the log corrects its own scroll after a slice
export function anchorsItself(): boolean {
  return typeof CSS !== "undefined" && typeof CSS.supports === "function" && CSS.supports("overflow-anchor", "auto");
}
