// The log follows the bottom only while the reader is there and not selecting or dragging; an upward wheel
// or finger lets go at once, and a hidden log keeps its place for when it comes back.

import { useEffect, useRef, type RefObject } from "react";

export function useFollowBottom(logRef: RefObject<HTMLDivElement | null>, { active, chat, startAtBottom }: { active: boolean; chat: unknown; startAtBottom: boolean }) {
  const stick = useRef(startAtBottom);
  const ownScroll = useRef(false); // a scroll we made, not the reader
  const dragging = useRef(false);
  const released = useRef(false); // the reader let go: an upward wheel or finger
  const savedTop = useRef<number | null>(null); // a hidden element forgets its scroll

  // back on screen, the place is put back; this runs before a jump's own scroll, so a search hit wins
  // the log's own window: in the pop-out that is the small window, whose frames and selection are its own
  const windowOf = (log: HTMLElement) => log.ownerDocument.defaultView ?? window;

  useEffect(() => {
    const log = logRef.current;
    if (!active || !log) return;
    ownScroll.current = true;
    log.scrollTop = stick.current ? log.scrollHeight : (savedTop.current ?? log.scrollTop);
    windowOf(log).requestAnimationFrame(() => (ownScroll.current = false));
  }, [active, logRef]);

  useEffect(() => {
    const log = logRef.current;
    if (!log || !stick.current || dragging.current) return;
    const sel = windowOf(log).getSelection();
    if (sel && !sel.isCollapsed && sel.anchorNode && log.contains(sel.anchorNode)) return;
    ownScroll.current = true;
    log.scrollTop = log.scrollHeight;
    windowOf(log).requestAnimationFrame(() => (ownScroll.current = false));
  }, [chat, logRef]);

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
