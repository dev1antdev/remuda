// useQuote watches the chat log's selection: text selected inside one agent turn gets a Quote button beside it, and Q
// quotes it too; the composer takes the quote. Nothing in a tile or the viewer: only a lens with a composer quotes.

import { useEffect, useRef, useState, type RefObject } from "react";
import { quoteKeyAsked } from "./quote";

// QuotePop is the selected text and where its button goes, in the window's coordinates
export type QuotePop = { text: string; x: number; y: number };

export function useQuote(logRef: RefObject<HTMLDivElement | null>, enabled: boolean, onQuote: (text: string) => void): QuotePop | null {
  const [pop, setPop] = useState<QuotePop | null>(null);
  const popRef = useRef(pop);
  popRef.current = pop;
  const quoteRef = useRef(onQuote);
  quoteRef.current = onQuote;
  useEffect(() => {
    const log = logRef.current;
    if (!enabled || !log) {
      setPop(null);
      return;
    }
    const doc = log.ownerDocument;
    const win = doc.defaultView ?? window;
    let frame = 0;
    const read = () => {
      win.cancelAnimationFrame(frame);
      frame = win.requestAnimationFrame(() => setPop(selectionIn(log)));
    };
    const onKey = (e: KeyboardEvent) => {
      const at = popRef.current;
      const press = { code: e.code, altKey: e.altKey, metaKey: e.metaKey, ctrlKey: e.ctrlKey, shiftKey: e.shiftKey, target: e.target as Element | null };
      if (!at || !quoteKeyAsked(press, true)) return;
      e.preventDefault();
      quoteRef.current(at.text);
    };
    doc.addEventListener("selectionchange", read);
    log.addEventListener("scroll", read, { passive: true });
    win.addEventListener("resize", read); // the text reflows under the selection, and the button must follow it
    doc.addEventListener("keydown", onKey);
    read();
    return () => {
      win.cancelAnimationFrame(frame);
      doc.removeEventListener("selectionchange", read);
      log.removeEventListener("scroll", read);
      win.removeEventListener("resize", read);
      doc.removeEventListener("keydown", onKey);
    };
  }, [logRef, enabled]);
  return pop;
}

// selectionIn is the selection when it lies inside one agent turn of the log and on screen, else nothing
function selectionIn(log: HTMLElement): QuotePop | null {
  const selection = log.ownerDocument.getSelection();
  if (!selection || selection.isCollapsed || selection.rangeCount === 0) return null;
  const turnOf = (node: Node | null) => (node?.nodeType === 1 ? (node as Element) : node?.parentElement)?.closest(".turn-assistant") ?? null;
  const turn = turnOf(selection.anchorNode);
  if (!turn || turn !== turnOf(selection.focusNode) || !log.contains(turn)) return null;
  const text = selection.toString();
  if (!text.trim()) return null;
  const rect = selection.getRangeAt(0).getBoundingClientRect();
  const box = log.getBoundingClientRect();
  if (rect.bottom < box.top || rect.top > box.bottom) return null;
  return { text, x: Math.min(rect.right, box.right), y: Math.max(rect.top, box.top) };
}
