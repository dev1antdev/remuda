// A long press on a phone: pointer events held still for `delay` fire once; a move, a lift or a cancel calls it off,
// and the click the lift sends afterwards is swallowed, so the row it was on neither opens nor toggles twice.

export type PressTarget = { pointerType: string; clientX: number; clientY: number };

export type PressHandlers = {
  onPointerDown: (e: PressTarget) => void;
  onPointerMove: (e: PressTarget) => void;
  onPointerUp: () => void;
  onPointerCancel: () => void;
  onClickCapture: (e: { preventDefault: () => void; stopPropagation: () => void }) => void;
  onContextMenu: (e: { preventDefault: () => void }) => void;
};

export const PRESS_DELAY = 500;
const SLOP = 8;

// longPress makes the handlers for one element; a mouse never presses, its right button has the menu
export function longPress(onPress: () => void, delay = PRESS_DELAY): PressHandlers {
  let timer: ReturnType<typeof setTimeout> | null = null;
  let fired = false;
  let touching = false;
  let x = 0;
  let y = 0;
  const cancel = () => {
    if (timer !== null) clearTimeout(timer);
    timer = null;
  };
  return {
    onPointerDown(e) {
      if (e.pointerType === "mouse") return;
      touching = true;
      fired = false;
      x = e.clientX;
      y = e.clientY;
      cancel();
      timer = setTimeout(() => {
        timer = null;
        fired = true;
        onPress();
      }, delay);
    },
    onPointerMove(e) {
      if (timer !== null && (Math.abs(e.clientX - x) > SLOP || Math.abs(e.clientY - y) > SLOP)) cancel();
    },
    onPointerUp() {
      cancel();
      touching = false;
    },
    onPointerCancel() {
      cancel();
      touching = false;
      fired = false;
    },
    onClickCapture(e) {
      if (!fired) return;
      fired = false;
      e.preventDefault();
      e.stopPropagation();
    },
    onContextMenu(e) {
      if (touching || fired) e.preventDefault();
    },
  };
}
