// Copy, from wherever the selection lives — the layer's DOM selection, the content selection, xterm's own —
// and the keys that belong to the page before xterm: Shift+Enter, the copy chords, Ctrl+V off a Mac.

import type { Terminal } from "@xterm/xterm";
import { copyText } from "@/lib/host";
import { MAC } from "./keys";
import type { Selection } from "./selection";

// trimRows drops each row's trailing run: a full-screen app pads its rows to the last column with real spaces,
// which a native terminal drops too.
export const trimRows = (raw: string) =>
  raw
    .split("\n")
    .map((l) => l.replace(/[ \t ]+$/, ""))
    .join("\n");

export function installCopy(term: Terminal, mount: HTMLElement, { layer, selection, sendInput, onCopied }: { layer: HTMLElement; selection: Selection; sendInput: (data: string) => void; onCopied: (chars: number) => void }): () => void {
  const selectedText = (): string => {
    const sel = window.getSelection();
    const raw = sel && !sel.isCollapsed && layer.contains(sel.anchorNode) ? sel.toString() : selection.active() ? selection.text() : term.hasSelection() ? term.getSelection() : "";
    return trimRows(raw);
  };
  const copySelection = (): boolean => {
    const text = selectedText();
    if (!text) return false;
    void copyText(text).then((ok) => ok && onCopied(text.length));
    return true;
  };
  // the native copy (the context menu, a phone's Copy): the selection may live in the layer, so the text is set by hand
  const onCopy = (e: ClipboardEvent) => {
    const text = selectedText();
    if (!text) return;
    e.clipboardData?.setData("text/plain", text);
    e.preventDefault();
    onCopied(text.length);
  };
  mount.addEventListener("copy", onCopy);

  // Shift+Enter is sent as Claude Code's newline chord Alt+Enter (ESC CR), every phase swallowed since Enter's
  // keypress would still emit "\r" and send the prompt; off a Mac, Ctrl+V must reach the browser's paste event.
  term.attachCustomKeyEventHandler((ev) => {
    if (ev.key === "Enter" && ev.shiftKey && !ev.ctrlKey && !ev.metaKey) {
      if (ev.type === "keydown") {
        ev.preventDefault();
        sendInput("\x1b\r");
      }
      return false;
    }
    if (ev.type !== "keydown") return true;
    if (!MAC && ev.key === "v" && ev.ctrlKey && !ev.altKey) return false;
    // copy: Cmd+C on a Mac, Ctrl+Shift+C elsewhere, Ctrl+C with a selection
    if ((ev.key === "c" || ev.key === "C") && (ev.metaKey || (ev.ctrlKey && (ev.shiftKey || term.hasSelection())))) {
      if (copySelection()) {
        ev.preventDefault();
        return false;
      }
      return !ev.metaKey;
    }
    return true;
  });
  return () => mount.removeEventListener("copy", onCopy);
}
