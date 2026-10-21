// The review's keys, while the lens is on screen and no field or dialog has them: j k walk the files, ↑ ↓ the
// lines of the open one, n p its hunks, c comments on the line, o opens the file there, s flips the two columns.
// By key position, not letter, so a Cyrillic layout works the same.

import { useEffect, useRef, type RefObject } from "react";

export const KEYS = [
  ["j", "k", "files"],
  ["↑", "↓", "lines"],
  ["n", "p", "hunks"],
  ["c", "", "comment"],
  ["o", "", "open"],
  ["s", "", "split"],
] as const;

export function useReviewKeys({ active, root, files, openFile, setOpenFile, onOpen, toggleSplit }: {
  active: boolean;
  root: RefObject<HTMLElement | null>;
  files: string[]; // in the order shown
  openFile: string | null;
  setOpenFile: (path: string | null) => void;
  onOpen: (path: string, line?: number) => void;
  toggleSplit: () => void;
}): { loaded: (path: string) => void } {
  const live = useRef({ files, openFile, setOpenFile, onOpen, toggleSplit });
  live.current = { files, openFile, setOpenFile, onOpen, toggleSplit };
  const wanted = useRef<string | null>(null); // a file opened by a key: its first line takes the focus once the diff is there
  useEffect(() => {
    if (!active) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.metaKey || event.ctrlKey || event.altKey || event.shiftKey) return;
      const target = event.target as HTMLElement | null;
      if (target?.closest("input, textarea, select, [contenteditable]") || document.querySelector(".modal-scrim, .fv, .sel-pop")) return;
      const box = root.current;
      if (!box) return;
      const { files, openFile, setOpenFile, onOpen, toggleSplit } = live.current;
      const lines = rowsOf(box, document.activeElement);
      const at = lines.findIndex((line) => line === document.activeElement);
      const go = (line: HTMLElement | null | undefined) => {
        if (!line) return;
        line.focus({ preventScroll: true });
        line.scrollIntoView({ block: "nearest" });
      };
      switch (event.code) {
        case "KeyJ":
        case "KeyK": {
          const index = openFile ? files.indexOf(openFile) : -1;
          const next = event.code === "KeyJ" ? files[index + 1] : index < 0 ? files[files.length - 1] : files[index - 1];
          if (!next) return;
          wanted.current = next;
          setOpenFile(next);
          break;
        }
        case "ArrowDown":
        case "ArrowUp": {
          if (lines.length === 0) return;
          const step = event.code === "ArrowDown" ? 1 : -1;
          go(lines[at < 0 ? 0 : Math.min(lines.length - 1, Math.max(0, at + step))]);
          break;
        }
        case "KeyN":
        case "KeyP": {
          const hunks = [...box.querySelectorAll<HTMLElement>(".review-file.open .review-hunk")];
          if (hunks.length === 0) return;
          const current = at < 0 ? -1 : hunks.findIndex((hunk) => hunk.contains(lines[at]!));
          const next = hunks[event.code === "KeyN" ? current + 1 : current < 0 ? hunks.length - 1 : current - 1];
          if (!next) return;
          go(lines.find((line) => next.contains(line)));
          break;
        }
        case "KeyC":
          if (at < 0) return;
          lines[at]!.click();
          break;
        case "KeyO": {
          if (!openFile) return;
          const line = at < 0 ? 0 : Number(lines[at]!.dataset.line);
          onOpen(openFile, line > 0 ? line : undefined);
          break;
        }
        case "KeyS":
          toggleSplit();
          break;
        case "Escape":
          if (at < 0) return;
          lines[at]!.blur();
          break;
        default:
          return;
      }
      event.preventDefault();
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [active, root]);
  // the file a key opened: once its diff is on screen, its head scrolls into view and its first line takes the focus
  const loaded = (path: string) => {
    if (wanted.current !== path) return;
    wanted.current = null;
    const box = root.current;
    if (!box) return;
    box.querySelector(".review-file.open .review-file-head")?.scrollIntoView({ block: "start" });
    rowsOf(box, null)[0]?.focus({ preventScroll: true });
  };
  return { loaded };
}

// rowsOf is one element per line of the open diff, in order: every line in the one-column view; in two columns the cell
// on the side the focus is on (the new one to begin with), the other side's where that one is empty
function rowsOf(box: HTMLElement, focus: Element | null): HTMLElement[] {
  const pairs = [...box.querySelectorAll<HTMLElement>(".review-file.open .fv-pair")];
  if (pairs.length === 0) return [...box.querySelectorAll<HTMLElement>(".review-file.open .fv-line.commentable")];
  const sideOf = (cell: Element) => (cell === cell.parentElement?.firstElementChild ? "old" : "new");
  const side = focus && focus.parentElement?.classList.contains("fv-pair") ? sideOf(focus) : "new";
  const rows: HTMLElement[] = [];
  for (const pair of pairs) {
    const cells = [...pair.querySelectorAll<HTMLElement>(".fv-line.commentable")];
    const cell = cells.find((candidate) => sideOf(candidate) === side) ?? cells[0];
    if (cell) rows.push(cell);
  }
  return rows;
}
