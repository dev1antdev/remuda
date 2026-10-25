// The log follows the bottom only while the reader is there and not selecting or dragging; an upward wheel
// or finger lets go at once, and a hidden log keeps its place for when it comes back. `mounted` counts the slices of a
// progressive mount (Chat), each of which pins the end again.

import { useEffect, useRef, type RefObject } from "react";

const SETTLE_FRAMES = 12;

export function useFollowBottom(logRef: RefObject<HTMLDivElement | null>, { active, chat, startAtBottom, mounted = 0 }: { active: boolean; chat: unknown; startAtBottom: boolean; mounted?: number }) {
  const stick = useRef(startAtBottom);
  const ownScroll = useRef(false); // a scroll we made, not the reader
  const dragging = useRef(false);
  const released = useRef(false); // the reader let go: an upward wheel or finger
  const savedTop = useRef<number | null>(null); // a hidden element forgets its scroll

  // back on screen, the place is put back; this runs before a jump's own scroll, so a search hit wins
  // the log's own window: in the pop-out that is the small window, whose frames and selection are its own
  const windowOf = (log: HTMLElement) => log.ownerDocument.defaultView ?? window;

  // toBottom scrolls to the end and keeps doing so, a frame at a time, until the end holds still: the turns near it are
  // skipped until they are on screen (content-visibility), so each scroll lays out a few more at their real heights and
  // moves the end; a dozen frames bound it, and an upward wheel meanwhile lets go through letGo as always
  const toBottom = (log: HTMLElement) => {
    const win = windowOf(log);
    ownScroll.current = true;
    const pin = (left: number) => {
      log.scrollTop = log.scrollHeight;
      win.requestAnimationFrame(() => {
        const away = log.scrollHeight - log.scrollTop - log.clientHeight;
        if (stick.current && away > 1 && left > 0) pin(left - 1);
        else ownScroll.current = false;
      });
    };
    pin(SETTLE_FRAMES);
  };

  useEffect(() => {
    const log = logRef.current;
    if (!active || !log) return;
    if (stick.current) {
      toBottom(log);
      return;
    }
    ownScroll.current = true;
    log.scrollTop = savedTop.current ?? log.scrollTop;
    windowOf(log).requestAnimationFrame(() => (ownScroll.current = false));
  }, [active, logRef]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    const log = logRef.current;
    if (!log || !stick.current || dragging.current) return;
    const sel = windowOf(log).getSelection();
    if (sel && !sel.isCollapsed && sel.anchorNode && log.contains(sel.anchorNode)) return;
    toBottom(log);
  }, [chat, mounted, logRef]); // eslint-disable-line react-hooks/exhaustive-deps

  const onScroll = () => {
    const log = logRef.current;
    if (!log || ownScroll.current) return;
    if (log.clientHeight > 0) savedTop.current = log.scrollTop; // a hidden log reads 0: not a place
    const d = log.scrollHeight - log.scrollTop - log.clientHeight;
    if (d < 1) released.current = false;
    stick.current = !released.current && d < 60;
  };

  useEffect(() => {
    const log = logRef.current;
    if (!log) return;
    // a gesture over a log that cannot scroll moves whatever is around it (the wall's grid), not this log
    const letGo = (up: boolean) => {
      if (log.scrollHeight <= log.clientHeight + 1) return;
      released.current = up;
      if (up) stick.current = false;
    };
    const onWheel = (e: WheelEvent) => letGo(e.deltaY < 0);
    let touchY = 0;
    const onTouchStart = (e: TouchEvent) => (touchY = e.touches[0]?.clientY ?? 0);
    const onTouchMove = (e: TouchEvent) => letGo((e.touches[0]?.clientY ?? 0) > touchY + 4);
    const onDown = () => (dragging.current = true);
    const onUp = () => (dragging.current = false);
    log.addEventListener("wheel", onWheel, { passive: true });
    log.addEventListener("touchstart", onTouchStart, { passive: true });
    log.addEventListener("touchmove", onTouchMove, { passive: true });
    const doc = log.ownerDocument;
    log.addEventListener("mousedown", onDown);
    doc.addEventListener("mouseup", onUp);
    return () => {
      log.removeEventListener("wheel", onWheel);
      log.removeEventListener("touchstart", onTouchStart);
      log.removeEventListener("touchmove", onTouchMove);
      log.removeEventListener("mousedown", onDown);
      doc.removeEventListener("mouseup", onUp);
    };
  }, [logRef]);

  return { onScroll, stick };
}
