// The terminal lens: the live terminal with its key bar under it, and the tools that sit in the tab
// strip's right slot — the size mode, the minimum width, the selection mode, the text size, the key bar.

import { useCallback, useEffect, useState } from "react";
import { actions } from "@/app/store";
import { isTouch, usePref } from "@/lib/prefs";
import type { Pane } from "@/features/roster/api";
import { Icon } from "@/ui";
import KeyBar from "./KeyBar";
import TerminalView from "./Terminal";
import type { SizeMode } from "./engine/types";
import { terminal } from "./state";

// minimum terminal widths on offer, in columns (0 = fit the screen)
const MIN_WIDTHS = [0, 120, 160, 200, 240, 320];

export function TerminalLens({ machineId, paneId, cols, rows, agent, active }: { machineId: string; paneId: string; cols: number; rows: number; agent: boolean; active: boolean }) {
  const [keybar] = usePref("keybar", isTouch);
  // a phone keeps the pane's own grid: resizing the shared pane to it would squeeze the desktop's view too
  const [sizeMode] = usePref<SizeMode>("sizeMode", isTouch ? "keep" : "fit");
  const [fontSize] = usePref("termFont", isTouch ? 12 : 13);
  const [minCols] = usePref("minCols", 0);
  const modifier = terminal.use((s) => s.modifier);
  const selectMode = terminal.use((s) => s.selectMode);
  const api = terminal.use((s) => s.api);
  const onOpenFile = useCallback((target: { path: string; line?: number; prefer?: "file" | "changes" }) => actions.openFile(machineId, paneId, target), [machineId, paneId]);
  const onCopied = useCallback((n: number) => actions.notify(`Copied ${n} character${n === 1 ? "" : "s"}`, { brief: true }), []);
  return (
    <>
      <TerminalView
        machineId={machineId}
        paneId={paneId}
        cols={cols}
        rows={rows}
        agent={agent}
        mode={sizeMode}
        minCols={minCols}
        fontSize={fontSize}
        modifier={modifier}
        selectMode={selectMode}
        onModifierUsed={() => terminal.set({ modifier: null })}
        onReady={(api) => terminal.set({ api })}
        onOpenFile={onOpenFile}
        onCopied={onCopied}
        active={active}
      />
      {keybar && <KeyBar term={api} modifier={modifier} setModifier={(modifier) => terminal.set({ modifier })} />}
    </>
  );
}

export function TerminalTools({ pane }: { pane: Pane }) {
  const [keybar, setKeybar] = usePref("keybar", isTouch);
  const [sizeMode, setSizeMode] = usePref<SizeMode>("sizeMode", isTouch ? "keep" : "fit");
  const [fontSize, setFontSize] = usePref("termFont", isTouch ? 12 : 13);
  const [minCols, setMinCols] = usePref("minCols", 0);
  const selectMode = terminal.use((s) => s.selectMode);
  const [widthMenu, setWidthMenu] = useState<{ x: number; y: number } | null>(null);
  const [widthMenuOpen, closeWidthMenu] = [!!widthMenu, () => setWidthMenu(null)];
  useDismissOnClick(widthMenuOpen, closeWidthMenu);
  return (
    <>
      <button
        type="button"
        className={`icon-btn kb-toggle${sizeMode === "keep" ? " on" : ""}`}
        title={sizeMode === "keep" ? `Keeping the pane's own size (${pane.cols}×${pane.rows}); click to resize it to this screen` : "Resizing the shared pane to this screen; click to keep its own size"}
        onClick={() => setSizeMode(sizeMode === "keep" ? "fit" : "keep")}
      >
        <Icon name={sizeMode === "keep" ? "minimize" : "maximize"} />
      </button>
      <button
        type="button"
        className={`icon-btn kb-toggle${minCols ? " on" : ""}`}
        disabled={sizeMode !== "fit"}
        aria-expanded={!!widthMenu}
        title={
          sizeMode !== "fit"
            ? "Minimum width applies when the pane is resized to this screen"
            : minCols
              ? `At least ${minCols} columns wide; a narrower screen scrolls sideways`
              : "Minimum width: make the terminal wider than the screen for diffs and tables, scrolling sideways"
        }
        onClick={(e) => {
          e.stopPropagation();
          const r = e.currentTarget.getBoundingClientRect();
          setWidthMenu(widthMenu ? null : { x: Math.min(r.left, window.innerWidth - 198), y: r.bottom });
        }}
      >
        <Icon name="move-horizontal" />
        {minCols ? <span className="kb-toggle-num">{minCols}</span> : null}
      </button>
      <button
        type="button"
        className={`icon-btn kb-toggle${selectMode ? " on" : ""}`}
        title={selectMode ? "Selection mode is on: drag selects text and copies it. Click to give the mouse back to the agent." : "Selection mode: drag to select and copy text (otherwise hold Option/Alt while dragging)"}
        onClick={() => terminal.set({ selectMode: !selectMode })}
      >
        <Icon name="text-select" />
      </button>
      <button type="button" className="icon-btn kb-toggle kb-font" title="Smaller text" onClick={() => setFontSize(Math.max(8, fontSize - 1))}>
        <Icon name="text-smaller" />
      </button>
      <button type="button" className="icon-btn kb-toggle kb-font" title="Larger text" onClick={() => setFontSize(Math.min(20, fontSize + 1))}>
        <Icon name="text-larger" />
      </button>
      <button type="button" className={`icon-btn kb-toggle${keybar ? " on" : ""}`} title="Key bar" aria-pressed={keybar} onClick={() => setKeybar(!keybar)}>
        <Icon name="keyboard" />
      </button>
      {widthMenu && (
        <div className="row-menu width-menu" role="menu" style={{ left: widthMenu.x, top: widthMenu.y }} onClick={(e) => e.stopPropagation()}>
          {MIN_WIDTHS.map((n) => (
            <button
              key={n}
              type="button"
              role="menuitemradio"
              aria-checked={n === minCols}
              className={n === minCols ? "on" : ""}
              onClick={() => {
                setMinCols(n);
                setWidthMenu(null);
              }}
            >
              {n ? `At least ${n} columns` : "Fit the screen"}
            </button>
          ))}
        </div>
      )}
    </>
  );
}

// a click anywhere, or Escape, closes the width menu
function useDismissOnClick(open: boolean, close: () => void) {
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && close();
    document.addEventListener("click", close);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("click", close);
      document.removeEventListener("keydown", onKey);
    };
  }, [open, close]);
}
