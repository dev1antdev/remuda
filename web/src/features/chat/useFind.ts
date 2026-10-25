// useFind marks every place the find bar's query occurs in the chat's log, through the CSS Custom Highlight
// API — no node changes, so a selection and the memoised turns stay as they are — keeps the current match on
// screen (revealRange: the log's own scroll handler then notes that the reader left the bottom, so the follow
// lets go) and tells the store how many there are. Where the API is missing, the current match's turn gets the
// outline a search jump gives it.

import { useEffect, useRef, type RefObject } from "react";
import { actions, app } from "@/app/store";
import { findRanges, MAX_MATCHES } from "./find";
import { revealRange } from "./reveal";

const ALL = "remuda-find";
const CURRENT = "remuda-find-current";

export function useFind(logRef: RefObject<HTMLDivElement | null>, { active, chat, mounted }: { active: boolean; chat: unknown; mounted: boolean }) {
  const open = app.use((s) => s.find !== null);
  const query = app.use((s) => s.find?.query ?? "");
  const at = app.use((s) => s.find?.at ?? 0);
  const ranges = useRef<Range[]>([]);
  const marked = useRef<Element | null>(null); // the turn outlined where no highlight registry exists

  // the matches, painted again whenever the query or the turns change, or the last of a long conversation's turns mount
  useEffect(() => {
    const log = logRef.current;
    if (!active || !open || !log) return;
    ranges.current = rangesIn(log, query);
    actions.foundMatches(ranges.current.length);
    const registry = highlights();
    if (registry) {
      const all = new Highlight();
      for (const range of ranges.current) all.add(range);
      registry.set(ALL, all);
      const current = ranges.current[at];
      registry.set(CURRENT, current ? new Highlight(current) : new Highlight());
    }
    return () => {
      registry?.delete(ALL);
      registry?.delete(CURRENT);
    };
  }, [active, open, query, at, chat, mounted, logRef]);

  // the current match comes on screen when it is chosen, not on every poll
  useEffect(() => {
    const log = logRef.current;
    if (!active || !open || !log) return;
    const current = ranges.current[at];
    const was = marked.current;
    marked.current = null;
    if (!current) {
      was?.classList.remove("turn-hit");
      return;
    }
    revealRange(log, current);
    if (highlights()) return;
    const turn = current.startContainer.parentElement?.closest(".turn") ?? null;
    if (was !== turn) was?.classList.remove("turn-hit");
    turn?.classList.add("turn-hit");
    marked.current = turn;
  }, [active, open, query, at, logRef]);

  useEffect(
    () => () => {
      marked.current?.classList.remove("turn-hit");
    },
    [],
  );
}

function highlights(): HighlightRegistry | null {
  return typeof Highlight === "function" && typeof CSS !== "undefined" && CSS.highlights ? CSS.highlights : null;
}

// rangesIn walks the log's text nodes — a folded turn renders none, so only what is on screen counts.
function rangesIn(root: HTMLElement, query: string): Range[] {
  if (!query) return [];
  const doc = root.ownerDocument;
  const walker = doc.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  const out: Range[] = [];
  for (let node = walker.nextNode(); node && out.length < MAX_MATCHES; node = walker.nextNode()) {
    const text = node.nodeValue ?? "";
    if (text.length < query.length) continue;
    for (const [start, end] of findRanges(text, query)) {
      const range = doc.createRange();
      range.setStart(node, start);
      range.setEnd(node, end);
      out.push(range);
      if (out.length >= MAX_MATCHES) break;
    }
  }
  return out;
}
