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
        if (ev.data instanceof ArrayBuffer) term.write(new Uint8Array(ev.data), () => { followBottom(); scheduleLayer(); });
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
            if (buf.length) term.write(buf, () => { followBottom(); scheduleLayer(); });
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

    // ---- the mouse stays ours ----
    // herdr enables mouse tracking in the attach stream (for its own
    // scrollback). If xterm honoured that, every drag would become mouse
    // reports and nothing could ever be selected. So the DECSET for the
    // tracking modes is intercepted here — xterm never enters tracking, a
    // plain drag selects — and the wheel is translated into the reports the
    // app asked for, so herdr still scrolls.
    const trackingModes = new Set([9, 1000, 1002, 1003]);
    let appMouse = 0; // the tracking mode the app wants; 0 = none
    let sgrMouse = false; // it asked for SGR (1006) reports
    const mouseModeHandler = (enable: boolean) => (params: (number | number[])[]) => {
      const rest: number[] = [];
      let touched = false;
      for (const p of params) {
        const n = Array.isArray(p) ? p[0]! : p;
        if (trackingModes.has(n)) {
          touched = true;
          appMouse = enable ? n : 0;
        } else {
          if (n === 1006) sgrMouse = enable;
          rest.push(n);
        }
      }
      if (!touched) return false; // nothing of ours: xterm handles it
      if (rest.length) term.write(`\x1b[?${rest.join(";")}${enable ? "h" : "l"}`); // the other modes still apply
      return true;
    };
    const decset = term.parser.registerCsiHandler({ prefix: "?", final: "h" }, mouseModeHandler(true));
    const decrst = term.parser.registerCsiHandler({ prefix: "?", final: "l" }, mouseModeHandler(false));

    const cellSize = () => {
      const core = (term as unknown as { _core?: { _renderService?: { dimensions?: { css?: { cell?: { width: number; height: number } } } } } })._core;
      const c = core?._renderService?.dimensions?.css?.cell;
      return { w: c?.width ?? term.options.fontSize! * 0.6, h: c?.height ?? term.options.fontSize! * 1.2 };
    };
    const onWheel = (e: WheelEvent) => {
      if (!appMouse) return; // no tracking wanted: xterm scrolls its own buffer
      const screen = term.element!.querySelector(".xterm-screen") as HTMLElement | null;
      if (!screen) return;
      e.preventDefault();
      e.stopPropagation();
      const r = screen.getBoundingClientRect();
      const { w, h } = cellSize();
      const col = Math.max(1, Math.min(term.cols, Math.floor((e.clientX - r.left) / w) + 1));
      const row = Math.max(1, Math.min(term.rows, Math.floor((e.clientY - r.top) / h) + 1));
      const up = e.deltaY < 0;
      const ticks = Math.min(3, Math.max(1, Math.round(Math.abs(e.deltaY) / 40)));
      const report = sgrMouse
        ? `\x1b[<${up ? 64 : 65};${col};${row}M`
        : "\x1b[M" + String.fromCharCode(32 + (up ? 64 : 65), 32 + col, 32 + row);
      sendInput(report.repeat(ticks));
    };
    mount.addEventListener("wheel", onWheel, { capture: true, passive: false });

    // ---- selection and copy ----
    // Two ways to select: Option/Alt+drag (xterm's own, even under the
    // agent's mouse tracking), and selection mode — a transparent text layer
    // over the grid whose selection is the browser's own, so it works where
    // xterm's mouse handling doesn't (an embedded browser in Remote
    // Development, a phone's long-press). Copy is Cmd+C / Ctrl+Shift+C, or
    // the native menu, routed through the host's clipboard when embedded.
    const layer = document.createElement("div");
    layer.className = "term-select";
    layer.hidden = true;
    term.element!.appendChild(layer);
    let layerTimer: ReturnType<typeof setTimeout> | null = null;

    const cellHeight = () => {
      const core = (term as unknown as { _core?: { _renderService?: { dimensions?: { css?: { cell?: { height: number } } } } } })._core;
      return core?._renderService?.dimensions?.css?.cell?.height ?? term.options.fontSize! * 1.2;
    };
    // the layer mirrors the visible rows as text, one row per line
    const paintLayer = () => {
      layerTimer = null;
      if (layer.hidden) return;
      const screen = term.element!.querySelector(".xterm-screen") as HTMLElement | null;
      if (screen) {
        layer.style.width = `${screen.offsetWidth}px`;
        layer.style.height = `${screen.offsetHeight}px`;
      }
      layer.style.fontSize = `${term.options.fontSize}px`;
      layer.style.lineHeight = `${cellHeight()}px`;
      const buf = term.buffer.active;
      const lines: string[] = [];
      for (let y = 0; y < term.rows; y++) {
        lines.push(buf.getLine(buf.viewportY + y)?.translateToString(true) ?? "");
      }
      const text = lines.join("\n");
      if (layer.textContent !== text) layer.textContent = text;
    };
    const scheduleLayer = () => {
      if (!layer.hidden && !layerTimer) layerTimer = setTimeout(paintLayer, 80);
    };
    const onScroll = term.onScroll(scheduleLayer);

    selectRef.current = (on) => {
      if (on) {
        layer.hidden = false;
        paintLayer();
      } else {
        layer.hidden = true;
        window.getSelection()?.removeAllRanges();
      }
    };

    // what is selected right now: the layer's DOM selection, else xterm's
    const selectedText = (): string => {
      const sel = window.getSelection();
      if (sel && !sel.isCollapsed && layer.contains(sel.anchorNode)) return sel.toString();
      return term.hasSelection() ? term.getSelection() : "";
    };
    const copySelection = (): boolean => {
      const text = selectedText();
      if (!text) return false;
      void copyText(text).then((ok) => ok && live.current.onCopied(text.length));
      return true;
    };
    // Cmd+C / the context menu: set the clipboard ourselves so the host
    // bridge (embedded browser) sees it too, not just the page's clipboard
    const onCopy = (e: ClipboardEvent) => {
      const text = selectedText();
      if (!text) return;
      e.clipboardData?.setData("text/plain", text);
      e.preventDefault();
      void copyText(text).then((ok) => ok && live.current.onCopied(text.length));
    };
    mount.addEventListener("copy", onCopy);

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
      onScroll.dispose();
      decset.dispose();
      decrst.dispose();
      mount.removeEventListener("wheel", onWheel, { capture: true });
      if (layerTimer) clearTimeout(layerTimer);
      mount.removeEventListener("copy", onCopy);
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
