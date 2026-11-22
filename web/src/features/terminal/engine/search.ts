// The find over the terminal's scrollback: xterm's own search addon marks every match in the page's amber and selects
// the current one. xterm holds the terminal's own rows — the attach paints the history first, the output after it is
// the program's — so the find looks through xterm's buffer, counts, steps and scrolls xterm, never the pane; when the
// buffer holds no match, useTerminalFind reads the older rows through /api/scrollback.

import { SearchAddon, type ISearchDecorationOptions, type ISearchOptions } from "@xterm/addon-search";
import type { IBufferLine, IBufferRange, Terminal as XTerm } from "@xterm/xterm";
import { findRanges } from "@/features/chat/find";
import { swatches, type Flavor, type Swatches } from "@/lib/theme";
import type { FoundCount, SearchApi } from "./types";

// RECOUNT_MS is how long output rests before the core find counts its query again.
const RECOUNT_MS = 100;

// samePlace says two selections start at one cell
const samePlace = (a: IBufferRange | undefined, b: IBufferRange | undefined): boolean => !!a && !!b && a.start.x === b.start.x && a.start.y === b.start.y;

// hexAlpha is a six-digit hex colour with an alpha, as #rrggbbaa, the form a decoration takes
export function hexAlpha(hex: string, alpha: number): string {
  const a = Math.round(Math.min(1, Math.max(0, alpha)) * 255)
    .toString(16)
    .padStart(2, "0");
  return hex + a;
}

// searchDecorations is the find bar's amber on the terminal: every match in a soft fill, the current one stronger
export function searchDecorations(c: Swatches): ISearchDecorationOptions {
  return {
    matchBackground: hexAlpha(c.yellow, 0.28),
    matchBorder: hexAlpha(c.yellow, 0.6),
    matchOverviewRuler: c.yellow,
    activeMatchBackground: hexAlpha(c.yellow, 0.6),
    activeMatchBorder: c.yellow,
    activeMatchColorOverviewRuler: c.peach,
  };
}

// installSearch loads the addon into the terminal; refresh paints the current find again (a flavour change). A match
// is selected the way the addon does it, and forgetSelection tells the engine's own selection tracker that this one
// is not the person's — else it would carry the mark to wherever that text seems to have moved after a repaint;
// selectionHeld says the person holds a selection of their own, which a recount never replaces.
export function installSearch(term: XTerm, flavor: () => Flavor, forgetSelection: () => void, selectionHeld: () => boolean = () => false): SearchApi & { refresh: () => void; onPainted: () => void; dispose: () => void } {
  let addon = new SearchAddon();
  // heard is the core find's ear: every count the addon gives, its own finds' and onPainted's recounts
  let heard: ((found: FoundCount) => void) | null = null;
  // unheard is the terminal as a core find's addon sees it: deaf to writes and resizes, so the addon never counts again
  // on its own from rows it cached before, nor replaces a selection the person holds; soon recounts instead
  const deaf = new Set<PropertyKey>(["onWriteParsed", "onResize"]);
  const unheard = new Proxy(term, { get: (target, key, receiver) => (deaf.has(key) ? () => ({ dispose() {} }) : Reflect.get(target, key, receiver)) });
  const load = () => {
    addon.activate(heard ? unheard : term);
    addon.onDidChangeResults((r) => heard?.({ total: Math.max(0, r.resultCount), at: Math.max(0, r.resultIndex) }));
  };
  load();
  // fresh replaces the addon before a find: its cache of the rows cannot know that a program rewrote a line in place
  // and put the cursor back (xterm reports no move), and a stale cache finds the screen as it was
  const fresh = () => {
    addon.dispose();
    addon = new SearchAddon();
    load();
  };
  let query = "";
  let pending: ReturnType<typeof setTimeout> | null = null;
  const options = (): ISearchOptions => ({ incremental: true, decorations: searchDecorations(swatches(flavor())) });
  // seek selects the next match, or the previous, and tells the engine the selection is not the person's
  const seek = (back = false): boolean => {
    const hit = back ? addon.findPrevious(query, options()) : addon.findNext(query, options());
    forgetSelection();
    return hit;
  };
  // restart starts the find over with a fresh addon from the top of the buffer
  const restart = () => {
    fresh();
    term.clearSelection();
  };
  // again finds the query with a fresh addon forward from the match selected — that match once more, or the next when it
  // is gone — then takes the step from there: back always (a fresh addon's own first step back would look forward
  // along the whole wrapped line), forward only when it found that very match again
  const again = (step: 0 | 1 | -1): boolean => {
    const was = term.getSelectionPosition();
    fresh();
    const hit = seek();
    if (!hit || step === 0) return hit;
    if (step > 0) return samePlace(term.getSelectionPosition(), was) ? seek() : hit;
    if (seek(true)) return true;
    // the addon's step back from a match on the buffer's last row never wraps round: from the buffer's end instead
    term.clearSelection();
    return seek(true);
  };
  let recount: ReturnType<typeof setTimeout> | null = null;
  // soon counts the core find's query again once output or a resize rests, where the screen stands, the viewport kept;
  // while the person holds a selection of their own the count waits for them to let it go
  const soon = () => {
    if (!heard || !query) return;
    if (recount) clearTimeout(recount);
    const count = () => {
      recount = null;
      if (!heard || !query) return;
      if (selectionHeld()) {
        recount = setTimeout(count, RECOUNT_MS);
        return;
      }
      const top = term.buffer.active.viewportY;
      if (!again(0)) heard({ total: 0, at: 0 });
      term.scrollToLine(top);
    };
    recount = setTimeout(count, RECOUNT_MS);
  };
  const resized = term.onResize(soon);
  const stop = () => {
    if (pending) clearTimeout(pending);
    if (recount) clearTimeout(recount);
    pending = recount = null;
  };
  const clear = () => {
    stop();
    heard = null;
    query = "";
    addon.clearDecorations();
    term.clearSelection();
  };
  return {
    find(next, step, hear) {
      stop();
      heard = hear;
      if (step !== 0 && next === query) {
        if (!again(step)) hear({ total: 0, at: 0 });
        return;
      }
      restart();
      query = next;
      if (!seek(step < 0)) hear({ total: 0, at: 0 });
    },
    clear,
    refresh() {
      if (query) again(0);
    },
    // onPainted hears a write landed, which may rewrite rows the core find read
    onPainted: soon,
    dispose() {
      stop();
      resized.dispose();
      addon.dispose();
    },
  };
}
