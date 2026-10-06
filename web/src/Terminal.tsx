import { useEffect, useRef } from "react";
import { Terminal as XTerm } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { uploadFiles } from "./api";
import { filesFrom, shellQuote } from "./attachments";
import { copyText } from "./host";
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

// Transport decision for this page: once WebSockets prove unusable, every
// terminal uses HTTP without trying again.
let preferHTTP = false;
let wsFailures = 0;
let wsRetry = 0;

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
  selectMode,
  onModifierUsed,
  onReady,
  onOpenFile,
  onCopied,
}: {
  machineId: string;
  paneId: string;
  cols: number; // the pane's own grid, 0 when unknown
  rows: number;
  mode: SizeMode;
  fontSize: number; // preferred size in fit mode
  modifier: Modifier;
  // selection mode: mouse reporting to the agent is paused locally, so a
  // plain drag selects text (otherwise Option/Alt+drag does)
  selectMode: boolean;
  onModifierUsed: () => void;
  onReady: (api: TermApi | null) => void;
  onOpenFile: (t: FileTarget) => void;
  onCopied: (chars: number) => void;
}) {
  const mountRef = useRef<HTMLDivElement>(null);
  // props the running terminal reads without being recreated
  const live = useRef({ cols, rows, mode, fontSize, modifier, onModifierUsed, onReady, onOpenFile, onCopied });
  live.current = { cols, rows, mode, fontSize, modifier, onModifierUsed, onReady, onOpenFile, onCopied };
  const applyRef = useRef<() => void>(() => {});
  const selectRef = useRef<(on: boolean) => void>(() => {});

  useEffect(() => {
    const mount = mountRef.current;
    if (!mount) return;

    const term = new XTerm({
      fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
      fontSize: live.current.fontSize,
      scrollback: 2000,
      allowProposedApi: true,
      macOptionClickForcesSelection: true, // Option+drag selects even while the agent tracks the mouse
      theme: { background: "#14120f", selectionBackground: "#4a4138" },
    });
    const fit = new FitAddon();
    term.loadAddon(fit);
    term.open(mount);

    let closed = false;
    let lastSize = "";

    // ---- transport: WebSocket, falling back to HTTP long-polling ----
    // JetBrains' embedded browser in Remote Development (and some proxies)
    // forward requests but not WebSockets: after two sockets that close
    // before they open, the page switches to the HTTP transport for good.
    type Transport = { input(data: string): void; resize(cols: number, rows: number): void; close(): void };
    let transport: Transport | null = null;
    const paneQuery = `machine=${encodeURIComponent(machineId)}&id=${encodeURIComponent(paneId)}`;
    const note = (text: string) => term.write(`\r\n\x1b[2m[herdr-ui] ${text}\x1b[0m\r\n`);

    // the PTY is shared by every viewer: never re-send an unchanged size
    const sendResize = () => {
      const size = `${term.cols}x${term.rows}`;
      if (size === lastSize) return;
      lastSize = size;
      transport?.resize(term.cols, term.rows);
    };
    const sendInput = (data: string) => transport?.input(data);
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

    // a fresh stream repaints from the replay: reset, then size it
    const onOpen = () => {
      lastSize = "";
      term.reset();
      sendResize();
    };

    const connectWS = (): Transport => {
      const proto = location.protocol === "https:" ? "wss" : "ws";
      let ws: WebSocket | null = new WebSocket(`${proto}://${location.host}/ws/pane?${paneQuery}&cols=${term.cols}&rows=${term.rows}`);
      ws.binaryType = "arraybuffer";
      let opened = false;
      const send = (obj: object) => {
        if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(obj));
      };
      ws.onopen = () => {
        opened = true;
        wsFailures = 0;
        wsRetry = 0;
        onOpen();
      };
      ws.onmessage = (ev) => {
        if (ev.data instanceof ArrayBuffer) term.write(new Uint8Array(ev.data), followBottom);
      };
      ws.onerror = () => ws?.close();
      ws.onclose = () => {
        if (closed || ws === null) return;
        if (!opened && ++wsFailures >= 2) {
          preferHTTP = true;
          note("WebSocket unavailable here — using the HTTP transport");
          transport = connectHTTP();
          return;
        }
        setTimeout(() => {
          if (!closed) transport = connectWS();
        }, Math.min(5000, 250 * 2 ** wsRetry++));
      };
      return {
        input: (data) => send({ type: "input", data }),
        resize: (cols, rows) => send({ type: "resize", cols, rows }),
        close: () => {
          const s = ws;
          ws = null;
          s?.close();
        },
      };
    };

    // HTTP: open a server-side stream, long-poll it for output, POST input
    // in small batches. A 410 means the stream expired: reopen and repaint.
    const connectHTTP = (): Transport => {
      let sid: string | null = null;
      let stopped = false;
      let pending = "";
      let flushTimer: ReturnType<typeof setTimeout> | null = null;

      const open = async () => {
        const res = await fetch(`/api/term/open?${paneQuery}&cols=${term.cols}&rows=${term.rows}`, { method: "POST" });
        if (!res.ok) throw new Error(`open failed (${res.status})`);
        sid = ((await res.json()) as { sid: string }).sid;
        onOpen();
      };
      const flush = async () => {
        flushTimer = null;
        if (!sid || !pending) return;
        const body = pending;
        pending = "";
        try {
          await fetch(`/api/term/input?sid=${sid}`, { method: "POST", body });
        } catch {
          pending = body + pending;
        }
      };
      const loop = async () => {
        try {
          await open();
        } catch (e) {
          if (!stopped) {
            note(`cannot open the terminal stream (${e instanceof Error ? e.message : String(e)}) — retrying`);
            setTimeout(loop, 2000);
          }
          return;
        }
        while (!stopped && sid) {
          try {
            const res = await fetch(`/api/term/read?sid=${sid}&wait=20`);
            if (res.status === 410) {
              sid = null;
              setTimeout(loop, 500); // expired: reopen
              return;
            }
            if (!res.ok) throw new Error(`read failed (${res.status})`);
            const buf = new Uint8Array(await res.arrayBuffer());
            if (buf.length) term.write(buf, followBottom);
            if (res.headers.get("X-Herdr-UI-End") === "1") {
              note("terminal detached");
              return;
            }
          } catch {
            if (stopped) return;
            await new Promise((r) => setTimeout(r, 1000));
          }
        }
      };
      void loop();
      return {
        input: (data) => {
          pending += data;
          if (!flushTimer) flushTimer = setTimeout(() => void flush(), 20);
        },
        resize: (cols, rows) => {
          if (sid) void fetch(`/api/term/resize?sid=${sid}&cols=${cols}&rows=${rows}`, { method: "POST" });
        },
        close: () => {
          stopped = true;
          if (sid) {
            void fetch(`/api/term/close?sid=${sid}`, { method: "DELETE", keepalive: true });
            sid = null;
          }
        },
      };
    };

    const onData = term.onData(input);

    // Copy on selection, like a terminal: the agent's mouse tracking makes
    // selecting deliberate anyway (Option+drag, or selection mode).
    let copyTimer: ReturnType<typeof setTimeout> | null = null;
    const onSel = term.onSelectionChange(() => {
      if (copyTimer) clearTimeout(copyTimer);
      copyTimer = setTimeout(() => {
        const text = term.getSelection();
        if (text) void copyText(text).then((ok) => ok && live.current.onCopied(text.length));
      }, 150);
    });

    // Selection mode pauses mouse reporting inside xterm only — the PTY
    // never sees these sequences — and restores whatever mode the agent had.
    let savedMouse: string | null = null;
    selectRef.current = (on) => {
      if (on) {
        if (savedMouse !== null) return;
        savedMouse = term.modes.mouseTrackingMode;
        term.write("\x1b[?1003l\x1b[?1002l\x1b[?1000l\x1b[?9l");
      } else if (savedMouse !== null) {
        const restore: Record<string, string> = { x10: "\x1b[?9h", vt200: "\x1b[?1000h", drag: "\x1b[?1002h", any: "\x1b[?1003h" };
        if (restore[savedMouse]) term.write(restore[savedMouse]!);
        savedMouse = null;
      }
    };

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

    // Files dropped or pasted onto the terminal upload into the pane's folder
    // and their paths are typed, shell-quoted, as if pasted — never sent.
    // Uploads are serialized so two drops don't interleave their paths.
    let uploads: Promise<void> = Promise.resolve();
    const typeFiles = (files: File[]) => {
      uploads = uploads.then(async () => {
        try {
          const ups = await uploadFiles(machineId, paneId, files);
          sendInput(ups.map((u) => shellQuote(u.path)).join(" ") + " ");
        } catch (e) {
          console.warn("upload failed:", e);
        }
      });
    };
    const onDragOver = (e: DragEvent) => {
      if (e.dataTransfer?.types.includes("Files")) {
        e.preventDefault();
        mount.classList.add("drag");
      }
    };
    const onDragLeave = () => mount.classList.remove("drag");
    const onDrop = (e: DragEvent) => {
      mount.classList.remove("drag");
      const files = filesFrom(e.dataTransfer);
      if (files.length === 0) return;
      e.preventDefault();
      typeFiles(files);
    };
    const onPaste = (e: ClipboardEvent) => {
      const files = filesFrom(e.clipboardData);
      if (files.length === 0) return; // plain text: xterm's own paste handles it
      e.preventDefault();
      e.stopPropagation();
      typeFiles(files);
    };
    mount.addEventListener("dragover", onDragOver);
    mount.addEventListener("dragleave", onDragLeave);
    mount.addEventListener("drop", onDrop);
    mount.addEventListener("paste", onPaste, true);

    // Debounced refit: a window-edge drag otherwise floods the agent with
    // resizes and Claude Code repaints its whole conversation per resize.
    let settle: ReturnType<typeof setTimeout> | null = null;
    const ro = new ResizeObserver(() => {
      if (settle) clearTimeout(settle);
      settle = setTimeout(applyGeometry, 120);
    });
    ro.observe(mount);

    transport = preferHTTP ? connectHTTP() : connectWS();
    live.current.onReady({ input, appCursor: () => term.modes.applicationCursorKeysMode });

    return () => {
      closed = true;
      live.current.onReady(null);
      onData.dispose();
      onSel.dispose();
      if (copyTimer) clearTimeout(copyTimer);
      linkProvider.dispose();
      mount.removeEventListener("dragover", onDragOver);
      mount.removeEventListener("dragleave", onDragLeave);
      mount.removeEventListener("drop", onDrop);
      mount.removeEventListener("paste", onPaste, true);
      ro.disconnect();
      if (settle) clearTimeout(settle);
      transport?.close();
      transport = null;
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

  useEffect(() => {
    selectRef.current(selectMode);
  }, [selectMode]);

  return <div ref={mountRef} className="term-mount" />;
}
