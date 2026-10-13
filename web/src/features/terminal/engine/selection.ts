// A selection that stays with the text. The app repaints the screen on every scroll, so the selection is
// kept in content rows and every row seen is remembered by its text; a settled screen is located in that
// memory and the selection drawn there, or hidden while the screen is somewhere unseen. A drag is ours
// from its first move (xterm reports a selection only on mouseup, so a wheel under a held button would go
// unseen), a drag past the top or bottom edge scrolls the app and grows the selection, and copy reads the
// remembered rows wherever the screen went.

import type { Terminal } from "@xterm/xterm";
import type { Modes } from "./modes";
import type { Point, Reporter } from "./mouse";

export type Pos = { row: number; col: number }; // content row, 0-based col

export type Selection = {
  active: () => boolean;
  text: () => string; // the selected text, on screen or remembered
  forget: () => void;
  onPainted: () => void; // the screen changed: locate it once it settles
  dragStart: (e: Point) => void;
  dragEnd: () => void;
  pointerAt: (p: Point) => void; // where the held button is
  dispose: () => void;
};

// relocate finds where the screen is in the content: the offset most rows agree on, among rows whose text is
// unique both on screen and in the memory; null when nothing is known. The app's own header and status rows
// sit on the same screen rows whatever the content and always vote for the old place, so a good share of the
// screen has to be recognised.
export function relocate(rows: string[], rowText: Map<number, string>, offset: number): number | null {
  const onScreen = new Map<string, number>();
  const twice = new Set<string>();
  rows.forEach((r, y) => {
    if (r.trim().length < 4) return;
    if (onScreen.has(r)) twice.add(r);
    else onScreen.set(r, y);
  });
  const inMemory = new Map<string, number>();
  for (const [row, text] of rowText) {
    if (!onScreen.has(text) || twice.has(text)) continue;
    if (inMemory.has(text)) twice.add(text);
    else inMemory.set(text, row);
  }
  const votes = new Map<number, number>();
  let total = 0;
  for (const [text, row] of inMemory) {
    if (twice.has(text)) continue;
    const o = row - onScreen.get(text)!;
    votes.set(o, (votes.get(o) ?? 0) + 1);
    total++;
  }
  let best: number | null = null;
  let bestVotes = 0;
  for (const [o, v] of votes) {
    if (v > bestVotes || (v === bestVotes && o === offset)) [best, bestVotes] = [o, v];
  }
  let unique = 0;
  for (const r of onScreen.keys()) if (!twice.has(r)) unique++;
  return best !== null && bestVotes >= 2 && bestVotes * 2 > total && bestVotes >= unique * 0.3 ? best : null;
}

// sliceRows is the text between two positions over the remembered rows, trailing spaces trimmed.
export function sliceRows(rowText: Map<number, string>, s: Pos, e: Pos): string {
  const lines: string[] = [];
  for (let r = s.row; r <= e.row; r++) {
    let t = rowText.get(r) ?? "";
    if (r === e.row) t = t.slice(0, e.col);
    if (r === s.row) t = t.slice(s.col);
    lines.push(t.replace(/[ \t ]+$/, ""));
  }
  return lines.join("\n");
}

const before = (a: Pos, b: Pos) => a.row < b.row || (a.row === b.row && a.col < b.col);

export function trackSelection(term: Terminal, reporter: Reporter, modes: Modes): Selection {
  let offset = 0; // content row = screen row + offset
  let attached = true; // the screen's place in the content is known
  let anchor: Pos | null = null;
  let head: Pos | null = null;
  const rowText = new Map<number, string>(); // content row → its text when last seen
  let applying = false; // our own term.select, not the user's
  let settleTimer: ReturnType<typeof setTimeout> | null = null;
  // the drag: its anchor, whether it moved, where the held button is, and the edge it is past
  let dragAnchor: Pos | null = null;
  let dragMoved = false;
  let pointer: Point | null = null;
  let edge: "up" | "down" | null = null;
  let edgeTimer: ReturnType<typeof setInterval> | null = null;

  const screenRows = (): string[] => {
    const buf = term.buffer.active;
    const out: string[] = [];
    for (let y = 0; y < term.rows; y++) out.push(buf.getLine(buf.baseY + y)?.translateToString(false) ?? "");
    return out;
  };
  const remember = (rows: string[]) => {
    for (let y = 0; y < rows.length; y++) rowText.set(y + offset, rows[y]!);
  };
  const ordered = (): [Pos, Pos] | null => (anchor && head ? (before(anchor, head) ? [anchor, head] : [head, anchor]) : null);
  const forget = () => {
    anchor = head = null;
    offset = 0;
    attached = true;
    rowText.clear();
  };
  const pointerPos = (): Pos | null => {
    if (!pointer) return null;
    const cell = reporter.cellAt(pointer);
    return cell ? { row: cell.row - 1 + offset, col: cell.col - 1 } : null;
  };
  const screenRect = () => (term.element?.querySelector(".xterm-screen") as HTMLElement | null)?.getBoundingClientRect() ?? null;

  // render draws the part of the selection that is on screen.
  const render = () => {
    const o = ordered();
    if (!o) return;
    const [s0, e0] = o;
    const top = offset;
    const bottom = offset + term.rows - 1;
    applying = true;
    try {
      if (!attached || e0.row < top || s0.row > bottom) {
        if (term.hasSelection()) term.clearSelection();
        return;
      }
      const s = s0.row < top ? { row: top, col: 0 } : s0;
      const e = e0.row > bottom ? { row: bottom, col: term.cols } : e0;
      const length = (e.row - s.row) * term.cols + (e.col - s.col);
      if (length > 0) term.select(s.col, term.buffer.active.baseY + s.row - offset, length);
      else if (term.hasSelection()) term.clearSelection();
    } finally {
      applying = false;
    }
  };
  // settled locates a settled screen: found in the memory, it is remembered and the selection drawn; else the selection waits unseen.
  const settled = () => {
    settleTimer = null;
    if (!anchor) return;
    const rows = screenRows();
    const found = relocate(rows, rowText, offset);
    if (found === null) {
      attached = false;
    } else {
      const moved = found !== offset;
      offset = found;
      attached = true;
      remember(rows);
      if (edge === "down" && head) head = { row: offset + term.rows - 1, col: term.cols };
      else if (edge === "up" && head) head = { row: offset, col: 0 };
      else if (moved && dragAnchor && pointer) {
        // a wheel under a held button: the head follows what is now under the pointer, as in a native terminal
        head = pointerPos() ?? head;
      }
    }
    render();
  };
  // the user's own selection changes: a drag, a double-click, a shift-click
  const onSelection = term.onSelectionChange(() => {
    if (applying) return;
    const pos = term.getSelectionPosition();
    // xterm drops its own selection on any input and on a buffer switch; ours stays until a new mousedown or typing
    if (!pos) return;
    const baseY = term.buffer.active.baseY;
    const start: Pos = { row: pos.start.y - baseY + offset, col: pos.start.x };
    const end: Pos = { row: pos.end.y - baseY + offset, col: pos.end.x };
    if (!anchor) remember(screenRows());
    // an end clipped at the screen's edge keeps the rows beyond it
    const o = ordered();
    const keepStart = o && o[0].row < offset && start.row === offset && start.col === 0;
    const keepEnd = o && o[1].row > offset + term.rows - 1 && end.row === offset + term.rows - 1;
    const s = keepStart ? o![0] : start;
    const e = keepEnd ? o![1] : end;
    if (dragAnchor && dragMoved) {
      anchor = dragAnchor;
      head = before(anchor, s) || (anchor.row === s.row && anchor.col === s.col) ? e : s;
    } else {
      anchor = s; // a word or a line (double, triple click), a shift-click
      head = e;
    }
  });

  const edgeTick = () => {
    if (!edge || !modes.appMouse()) return;
    const r = screenRect();
    if (!r) return;
    const cell = reporter.cellAt({ clientX: r.left + 1, clientY: edge === "down" ? r.bottom - 1 : r.top + 1 });
    if (cell) reporter.scroll(edge === "down" ? 1 : -1, false, cell);
  };
  const onDragMove = (e: MouseEvent) => {
    pointer = { clientX: e.clientX, clientY: e.clientY };
    dragMoved = true;
    if (!edge) {
      head = pointerPos() ?? head;
      render();
    }
    const r = screenRect();
    if (!r) return;
    const next: typeof edge = e.clientY > r.bottom ? "down" : e.clientY < r.top ? "up" : null;
    if (next === edge) return;
    edge = next;
    if (edgeTimer) clearInterval(edgeTimer);
    edgeTimer = edge ? setInterval(edgeTick, 90) : null;
  };
  const dragStart = (e: Point) => {
    forget();
    const cell = reporter.cellAt(e);
    dragAnchor = cell ? { row: cell.row - 1, col: cell.col - 1 } : null;
    dragMoved = false;
    pointer = { clientX: e.clientX, clientY: e.clientY };
    if (dragAnchor) {
      anchor = head = dragAnchor;
      remember(screenRows());
    }
    document.addEventListener("mousemove", onDragMove);
  };
  const dragEnd = () => {
    if (!dragMoved) forget(); // a click; xterm's word or line may still follow
    dragAnchor = null;
    pointer = null;
    edge = null;
    if (edgeTimer) clearInterval(edgeTimer);
    edgeTimer = null;
    document.removeEventListener("mousemove", onDragMove);
  };

  return {
    active: () => anchor !== null,
    text: () => {
      const o = ordered();
      return o ? sliceRows(rowText, o[0], o[1]) : "";
    },
    forget,
    onPainted: () => {
      if (!anchor) return;
      if (settleTimer) clearTimeout(settleTimer);
      settleTimer = setTimeout(settled, 40);
    },
    dragStart,
    dragEnd,
    pointerAt: (p) => {
      pointer = { clientX: p.clientX, clientY: p.clientY };
    },
    dispose: () => {
      onSelection.dispose();
      dragEnd();
      if (settleTimer) clearTimeout(settleTimer);
    },
  };
}
