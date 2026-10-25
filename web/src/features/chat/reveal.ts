// Bringing a turn or a match into view in a log whose off-screen turns are skipped (content-visibility: auto): a skipped
// turn stands at an estimated height until it is near the viewport, so one scroll lands near the target and the next
// frame, with the real heights in, puts it where it was asked. The geometry is pure; the two reveals drive the DOM.

export type Box = { top: number; bottom: number; height: number };

// centerDelta is how far to scroll a box so a hit sits in its middle
export function centerDelta(hit: Box, box: Box): number {
  return hit.top - box.top - box.height / 2 + hit.height / 2;
}

// inView says whether a hit lies wholly inside a box
export function inView(hit: Box, box: Box): boolean {
  return hit.top >= box.top && hit.bottom <= box.bottom;
}

// unmeasured is a rect of nothing: the text sits in a skipped turn and has no layout yet
export function unmeasured(hit: Box): boolean {
  return hit.height === 0 && hit.top === 0 && hit.bottom === 0;
}

const PASSES = 2;

// revealTurn scrolls a turn to the top of the log and, on the next frames, again where the heights around it settled elsewhere
export function revealTurn(log: HTMLElement, turn: Element): void {
  const frame = log.ownerDocument.defaultView?.requestAnimationFrame ?? ((f: () => void) => setTimeout(f, 16));
  const pass = (left: number) => {
    turn.scrollIntoView({ block: "start" });
    if (left > 0) frame(() => pass(left - 1));
  };
  pass(PASSES);
}

// revealRange puts a match in the middle of the log, unless it is in view already; a match in a skipped turn is first
// scrolled to by its turn, then centred once the turn has its layout
export function revealRange(log: HTMLElement, range: Range): void {
  const frame = log.ownerDocument.defaultView?.requestAnimationFrame ?? ((f: () => void) => setTimeout(f, 16));
  const pass = (left: number) => {
    const hit = range.getBoundingClientRect();
    const box = log.getBoundingClientRect();
    if (unmeasured(hit)) {
      range.startContainer.parentElement?.closest(".turn")?.scrollIntoView({ block: "center" });
    } else if (!inView(hit, box)) {
      log.scrollTop += centerDelta(hit, box);
    } else {
      return;
    }
    if (left > 0) frame(() => pass(left - 1));
  };
  pass(PASSES);
}
