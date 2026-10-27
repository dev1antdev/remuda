// The terminal lens: the live terminal with its key bar under it, and the tools that sit in the tab
// strip's right slot — the size mode, the minimum width, the selection mode, the text size, the key bar.

import { useFlavor, useMono } from "@/app/theme";
import { useCallback } from "react";
import { actions } from "@/app/store";
import { monoStack } from "@/lib/look";
import { isTouch, usePref } from "@/lib/prefs";
import type { Pane } from "@/features/roster/api";
import { Icon, type MenuItem, Tip } from "@/ui";
import KeyBar from "./KeyBar";
import TerminalView from "./Terminal";
import type { SizeMode } from "./engine/types";
import { terminal } from "./state";
import { useTerminalFind } from "./useTerminalFind";

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
  useTerminalFind(api, active, machineId, paneId);
  const flavor = useFlavor();
  const mono = useMono();
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
        flavor={flavor}
        fontFamily={monoStack(mono)}
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

export function TerminalTools() {
  const [keybar, setKeybar] = usePref("keybar", isTouch);
  return (
    <Tip text={keybar ? "Hide the key bar" : "Key bar: Esc, Tab, Ctrl, the arrows and more, for a screen without them"}>
      <button type="button" className={`icon-btn kb-toggle${keybar ? " on" : ""}`} aria-label="Key bar" aria-pressed={keybar} onClick={() => setKeybar(!keybar)}>
        <Icon name="keyboard" />
      </button>
    </Tip>
  );
}

// useTerminalViewItems is the terminal's part of the View menu: the size mode, the minimum width, selection, text size, the key bar
export function useTerminalViewItems(pane: Pane): MenuItem[] {
  const [keybar, setKeybar] = usePref("keybar", isTouch);
  const [sizeMode, setSizeMode] = usePref<SizeMode>("sizeMode", isTouch ? "keep" : "fit");
  const [fontSize, setFontSize] = usePref("termFont", isTouch ? 12 : 13);
  const [minCols, setMinCols] = usePref("minCols", 0);
  const selectMode = terminal.use((s) => s.selectMode);
  return [
    { heading: "Size" },
    { label: "Resize the pane to this screen", hint: "herdr follows the smallest attached client", checked: sizeMode === "fit", onSelect: () => setSizeMode("fit") },
    { label: `Keep the pane's own size (${pane.cols}×${pane.rows})`, hint: "a smaller font to fit the width, scrolling when it cannot", checked: sizeMode === "keep", onSelect: () => setSizeMode("keep") },
    { heading: "Minimum width" },
    ...MIN_WIDTHS.map((n) => ({ label: n ? `At least ${n} columns` : "Fit the screen", hint: n ? (n === minCols ? "a narrower screen scrolls sideways" : undefined) : undefined, checked: n === minCols, disabled: sizeMode !== "fit", onSelect: () => setMinCols(n) })),
    { sep: true },
    { label: "Selection mode", icon: "text-select", hint: selectMode ? "drag selects and copies; off gives the mouse back to the agent" : "drag selects and copies text (or hold Option/Alt while dragging)", checked: selectMode, onSelect: () => terminal.set({ selectMode: !selectMode }) },
    { sep: true },
    { label: "Smaller text", icon: "text-smaller", hint: `${fontSize} px`, keepOpen: true, disabled: fontSize <= 8, onSelect: () => setFontSize(Math.max(8, fontSize - 1)) },
    { label: "Larger text", icon: "text-larger", hint: `${fontSize} px`, keepOpen: true, disabled: fontSize >= 20, onSelect: () => setFontSize(Math.min(20, fontSize + 1)) },
    { sep: true },
    { label: "Key bar", icon: "keyboard", hint: "Esc, Tab, Ctrl, the arrows and more", checked: keybar, onSelect: () => setKeybar(!keybar) },
  ];
}
