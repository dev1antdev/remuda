// TerminalView is the live terminal for one herdr pane: a mount for the engine (engine/engine.ts), which
// lives for as long as the pane is shown; the props re-apply without reconnecting.

import { useEffect, useRef } from "react";
import type { OpenFile } from "@/features/files/target";
import { isTouch } from "@/lib/prefs";
import { createEngine, type Engine } from "./engine/engine";
import type { Modifier, SizeMode, TermApi } from "./engine/types";

export default function TerminalView({ machineId, paneId, agent, cols, rows, mode, minCols, fontSize, modifier, selectMode, onModifierUsed, onReady, onOpenFile, onCopied, active = true }: {
  machineId: string;
  paneId: string;
  cols: number; // the pane's own grid, 0 when unknown
  rows: number;
  agent: boolean; // an agent's pane: file paths and URLs are links; in a shell's pane only a Cmd/Ctrl-click opens a URL
  mode: SizeMode;
  minCols: number; // fit mode: never narrower than this many columns (0 = off)
  fontSize: number; // preferred size in fit mode
  modifier: Modifier;
  selectMode: boolean; // a transparent text layer over the grid whose selection is the browser's own (a phone's long-press)
  onModifierUsed: () => void;
  onReady: (api: TermApi | null) => void;
  onOpenFile: OpenFile;
  onCopied: (chars: number) => void;
  active?: boolean; // the lens on screen; hidden, the attach stays and output keeps arriving
}) {
  const mountRef = useRef<HTMLDivElement>(null);
  const live = useRef({ agent, cols, rows, mode, minCols, fontSize, modifier, onModifierUsed, onReady, onOpenFile, onCopied });
  live.current = { agent, cols, rows, mode, minCols, fontSize, modifier, onModifierUsed, onReady, onOpenFile, onCopied };
  const engine = useRef<Engine | null>(null);

  useEffect(() => {
    const mount = mountRef.current;
    if (!mount) return;
    const e = createEngine(mount, machineId, paneId, () => live.current);
    engine.current = e;
    return () => {
      e.dispose();
      engine.current = null;
    };
  }, [machineId, paneId]);

  // geometry props re-apply without reconnecting; the pane's own grid only matters in keep mode
  const keepCols = mode === "keep" ? cols : 0;
  const keepRows = mode === "keep" ? rows : 0;
  useEffect(() => {
    engine.current?.apply();
  }, [keepCols, keepRows, mode, minCols, fontSize]);

  useEffect(() => {
    engine.current?.select(selectMode);
  }, [selectMode]);

  // back on screen (not the first show): measured again and, with a keyboard, focused — a touch screen's keyboard must not pop
  const wasActive = useRef(active);
  useEffect(() => {
    const was = wasActive.current;
    wasActive.current = active;
    if (!active || was) return;
    engine.current?.apply();
    if (!isTouch) engine.current?.focus();
  }, [active]);

  return <div ref={mountRef} className="term-mount" />;
}
