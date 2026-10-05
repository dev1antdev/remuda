import { useEffect, useRef } from "react";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { findPaths } from "./paths";
import type { FileTarget } from "./FileViewer";

export type Modifier = "ctrl" | "alt" | null;

// How this viewer relates to the shared pane's size:
//  fit  — the pane is resized to this screen (herdr follows the smallest
//         attached client; the desktop's own view shrinks with a phone)
//  keep — attach at the pane's own grid and shrink the font to show it
//         whole; too wide for the screen, it scrolls instead
export type SizeMode = "fit" | "keep";

// What the key bar needs from a live terminal: a way to type a sequence as
// if from the keyboard (through the same onData → socket path), and the
// cursor-key mode (DECCKM decides ESC O A vs ESC [ A). A one-shot Ctrl/Alt
// lives in React state above both; input() applies and consumes it.
export type TermApi = {
  input: (seq: string) => void;
  appCursor: () => boolean;
};

const MIN_FONT = 8;

// applyModifier encodes a single printable character under a one-shot
// Ctrl (control code) or Alt (ESC prefix); anything else passes through.
export function applyModifier(mod: Modifier, seq: string): string {
  if (!mod || seq.length !== 1 || seq < " " || seq > "~") return seq;
  return mod === "ctrl" ? String.fromCharCode(seq.toUpperCase().charCodeAt(0) & 0x1f) : "\x1b" + seq;
}

// Live terminal for one herdr pane: raw pane bytes arrive as binary WS frames,
// keystrokes go back as JSON. herdr keeps the real scrollback and mouse
// reporting; the local buffer is small and its scrollbar is hidden (an overlay
// scrollbar would paint over the last column).
export default function TerminalView({
  machineId,
  paneId,
  cols,
  rows,
  mode,
  fontSize,
  modifier,
  onModifierUsed,
  onReady,
  onOpenFile,
}: {
  machineId: string;
  paneId: string;
  cols: number; // the pane's own grid, 0 when unknown
  rows: number;
  mode: SizeMode;
  fontSize: number; // preferred size in fit mode
  modifier: Modifier;
  onModifierUsed: () => void;
  onReady: (api: TermApi | null) => void;
  onOpenFile: (t: FileTarget) => void;
}) {
  const mountRef = useRef<HTMLDivElement>(null);
  // props the running terminal reads without being recreated
  const live = useRef({ cols, rows, mode, fontSize, modifier, onModifierUsed, onReady, onOpenFile });
  live.current = { cols, rows, mode, fontSize, modifier, onModifierUsed, onReady, onOpenFile };
  const applyRef = useRef<() => void>(() => {});

  useEffect(() => {
    const mount = mountRef.current;
    if (!mount) return;

    const term = new XTerm({
      fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
      fontSize: live.current.fontSize,
      scrollback: 2000,
      allowProposedApi: true,
      theme: { background: "#14120f", selectionBackground: "#4a4138" },
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(mount);

    let ws: WebSocket | null = null;
    let closed = false;
    let retry = 0;
    let lastSize = "";

    const send = (obj: object) => {
      if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(obj));
    };
    // the PTY is shared by every viewer: never re-send an unchanged size
    const sendResize = () => {
      const size = `${term.cols}x${term.rows}`;
      if (size === lastSize) return;
      lastSize = size;
      send({ type: "resize", cols: term.cols, rows: term.rows });
    };
    const sendInput = (data: string) => send({ type: "input", data });
    const input = (seq: string) => {
      const m = live.current.modifier;
      sendInput(applyModifier(m, seq));
      if (m) live.current.onModifierUsed();
    };

    // Cell width in px per font px, measured once from the renderer.
    const cellRatio = () => {
      const core = (term as unknown as { _core?: { _renderService?: { dimensions?: { css?: { cell?: { width: number } } } } } })._core;
      const w = core?._renderService?.dimensions?.css?.cell?.width;
      return w && w > 0 ? w / term.options.fontSize! : 0.6;
    };

    // applyGeometry sizes the grid: fit → to the mount; keep → the pane's own
    // grid, with the font shrunk until the width fits (down to MIN_FONT, then
    // the mount scrolls).
    const applyGeometry = () => {
      const g = live.current;
      if (g.mode === "keep" && g.cols > 0 && g.rows > 0) {
        const avail = mount.clientWidth - 8;
        const size = Math.max(MIN_FONT, Math.min(g.fontSize, Math.floor(avail / (g.cols * cellRatio()))));
        if (term.options.fontSize !== size) term.options.fontSize = size;
        if (term.cols !== g.cols || term.rows !== g.rows) term.resize(g.cols, g.rows);
        mount.classList.add("keep");
      } else {
        if (term.options.fontSize !== g.fontSize) term.options.fontSize = g.fontSize;
        mount.classList.remove("keep");
        fit.fit();
      }
      sendResize();
    };
    applyRef.current = applyGeometry;
    applyGeometry();

    // In keep mode the grid may be taller than the screen: follow the bottom
    // (the agent's input line) unless the reader scrolled up — one layout
    // read per animation frame, not per chunk.
    let followQueued = false;
    const followBottom = () => {
      if (followQueued || !mount.classList.contains("keep")) return;
      followQueued = true;
      requestAnimationFrame(() => {
        followQueued = false;
        const nearBottom = mount.scrollHeight - mount.scrollTop - mount.clientHeight < 80;
        if (nearBottom) mount.scrollTop = mount.scrollHeight;
      });
    };

    const connect = () => {
      if (closed) return;
      const proto = location.protocol === "https:" ? "wss" : "ws";
      ws = new WebSocket(
        `${proto}://${location.host}/ws/pane?machine=${encodeURIComponent(machineId)}&id=${encodeURIComponent(paneId)}&cols=${term.cols}&rows=${term.rows}`,
      );
      ws.binaryType = "arraybuffer";
      ws.onopen = () => {
        retry = 0;
        lastSize = "";
        term.reset();
        sendResize();
      };
      ws.onmessage = (ev) => {
        if (ev.data instanceof ArrayBuffer) term.write(new Uint8Array(ev.data), followBottom);
      };
      ws.onclose = () => {
        if (closed) return;
        setTimeout(connect, Math.min(5000, 250 * 2 ** retry++));
      };
      ws.onerror = () => ws?.close();
    };

    const onData = term.onData(input);

    // Keys xterm gets wrong for agent TUIs: Shift+Enter is a newline in
    // Claude Code only as the Alt+Enter chord (ESC CR); Ctrl+V must reach the
    // browser's paste event instead of becoming 0x16. The handler sees
    // keydown, keypress AND keyup — Enter's keypress would still emit "\r"
    // and send the prompt, so every phase of the chord is swallowed.
    term.attachCustomKeyEventHandler((ev) => {
      if (ev.key === "Enter" && ev.shiftKey && !ev.ctrlKey && !ev.metaKey) {
        if (ev.type === "keydown") {
          ev.preventDefault();
          sendInput("\x1b\r");
        }
        return false;
      }
      if (ev.type !== "keydown") return true;
      if (ev.key === "v" && ev.ctrlKey && !ev.altKey) return false;
      if (ev.key === "c" && ev.ctrlKey && term.hasSelection()) return false; // copy
      return true;
    });

    // File paths the agent prints become links: a click opens the file where
    // it lives (this machine or the remote one), at the :line if given.
    const linkProvider = term.registerLinkProvider({
      provideLinks(y, callback) {
        const row = term.buffer.active.getLine(y - 1);
        if (!row) return callback(undefined);
        const links = findPaths(row.translateToString(true)).map((m) => ({
          range: { start: { x: m.start + 1, y }, end: { x: m.end, y } },
          text: m.text,
          decorations: { pointerCursor: true, underline: true },
          activate: () => live.current.onOpenFile({ path: m.path, line: m.line }),
        }));
        callback(links.length ? links : undefined);
      },
    });

    // Debounced refit: a window-edge drag otherwise floods the agent with
    // resizes and Claude Code repaints its whole conversation per resize.
    let settle: ReturnType<typeof setTimeout> | null = null;
    const ro = new ResizeObserver(() => {
      if (settle) clearTimeout(settle);
      settle = setTimeout(applyGeometry, 120);
    });
    ro.observe(mount);

    connect();
    live.current.onReady({ input, appCursor: () => term.modes.applicationCursorKeysMode });

    return () => {
      closed = true;
      live.current.onReady(null);
      onData.dispose();
      linkProvider.dispose();
      ro.disconnect();
      if (settle) clearTimeout(settle);
      ws?.close();
      term.dispose();
    };
  }, [machineId, paneId]);

  // geometry props changed: re-apply without reconnecting. The pane's own
  // grid only matters in keep mode.
  const keepCols = mode === "keep" ? cols : 0;
  const keepRows = mode === "keep" ? rows : 0;
  useEffect(() => {
    applyRef.current();
  }, [keepCols, keepRows, mode, fontSize]);

  return <div ref={mountRef} className="term-mount" />;
}
