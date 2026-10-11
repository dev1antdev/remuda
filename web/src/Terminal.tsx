import { useEffect, useRef } from "react";
import { Terminal as XTerm, type ILink } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { uploadFiles } from "./api";
import { filesFrom, shellQuote } from "./attachments";
import { copyText, openUrl } from "./host";
import { isTouch } from "./prefs";
import { findPaths, findUrls } from "./paths";
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
const MAC = /Mac|iPhone|iPad/.test(navigator.platform);

// Transport decision for this page: once WebSockets prove unusable, every
// terminal uses HTTP without trying again.
let preferHTTP = false;
let wsFailures = 0;
let wsRetry = 0;

// applyModifier encodes a single printable character under a one-shot
// Ctrl (control code) or Alt (ESC prefix), and an arrow as the modified CSI
// form (ESC [ 1 ; 5 A for Ctrl, ; 3 for Alt); anything else passes through
// unchanged, and the caller keeps the modifier armed.
export function applyModifier(mod: Modifier, seq: string): string {
  if (!mod) return seq;
  const arrow = /^\x1b[[O]([ABCD])$/.exec(seq);
  if (arrow) return `\x1b[1;${mod === "ctrl" ? 5 : 3}${arrow[1]}`;
  if (seq.length !== 1 || seq < " " || seq > "~") return seq;
  return mod === "ctrl" ? String.fromCharCode(seq.toUpperCase().charCodeAt(0) & 0x1f) : "\x1b" + seq;
}

// A DECSET that mixes a mouse-tracking mode with other modes is split in
// two before xterm sees it — the others first — so the handler below only
// ever meets pure sequences and never has to re-inject (a re-injected
// sequence would land behind the rest of the chunk, out of order).
const TRACKING = new Set([9, 1000, 1002, 1003]);
const DECSET = /\x1b\[\?([\d;]+)([hl])/g;
export function splitTracking(s: string): string {
  if (!s.includes("\x1b[?")) return s;
  return s.replace(DECSET, (all, params: string, final: string) => {
    if (!params.includes(";")) return all;
    const ns = params.split(";");
    const ours = ns.filter((n) => TRACKING.has(Number(n)));
    if (ours.length === 0 || ours.length === ns.length) return all;
    const theirs = ns.filter((n) => !TRACKING.has(Number(n)));
    return `\x1b[?${theirs.join(";")}${final}\x1b[?${ours.join(";")}${final}`;
  });
}

// Live terminal for one herdr pane: raw pane bytes arrive as binary WS frames,
// keystrokes go back as JSON. herdr keeps the real scrollback and mouse
// reporting; the local buffer is small and its scrollbar is hidden (an overlay
// scrollbar would paint over the last column).
export default function TerminalView({
  machineId,
  paneId,
  agent,
  cols,
  rows,
  mode,
  minCols,
  fontSize,
  modifier,
  selectMode,
  onModifierUsed,
  onReady,
  onOpenFile,
  onCopied,
  active = true,
}: {
  machineId: string;
  paneId: string;
  cols: number; // the pane's own grid, 0 when unknown
  rows: number;
  // an agent's pane: file paths and links are clickable. A shell's pane
  // belongs to whatever runs in it — clicks go to the app, only URLs with
  // Cmd/Ctrl held are links.
  agent: boolean;
  mode: SizeMode;
  minCols: number; // fit mode: never narrower than this many columns (0 = off)
  fontSize: number; // preferred size in fit mode
  modifier: Modifier;
  // selection mode: a transparent text layer over the grid whose selection
  // is the browser's own — for a phone's long-press and the places where
  // xterm's own selection does not work
  selectMode: boolean;
  onModifierUsed: () => void;
  onReady: (api: TermApi | null) => void;
  onOpenFile: (t: FileTarget) => void;
  onCopied: (chars: number) => void;
  active?: boolean; // the lens on screen; hidden, the attach stays and output keeps arriving
}) {
  const mountRef = useRef<HTMLDivElement>(null);
  // props the running terminal reads without being recreated
  const live = useRef({ agent, cols, rows, mode, minCols, fontSize, modifier, onModifierUsed, onReady, onOpenFile, onCopied });
  live.current = { agent, cols, rows, mode, minCols, fontSize, modifier, onModifierUsed, onReady, onOpenFile, onCopied };
  const applyRef = useRef<() => void>(() => {});
  const focusRef = useRef<() => void>(() => {});
  const selectRef = useRef<(on: boolean) => void>(() => {});

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

    let closed = false;
    let lastSize = "";

    // ---- transport: WebSocket, falling back to HTTP long-polling ----
    // Some proxies forward requests but not WebSockets: after two sockets
    // that close before they open while the same request works as plain
    // HTTP, the page switches to the HTTP transport for good.
    type Transport = { input(data: string): void; resize(cols: number, rows: number): void; close(): void };
    let transport: Transport | null = null;
    const paneQuery = `machine=${encodeURIComponent(machineId)}&id=${encodeURIComponent(paneId)}`;
    const note = (text: string) => term.write(`\r\n\x1b[2m[remuda] ${text}\x1b[0m\r\n`);

    // the PTY is shared by every viewer: never re-send an unchanged size.
    // The connect request already carries the size, so lastSize is set
    // there and a fresh connection sends nothing until the grid changes.
    const sendResize = () => {
      const size = `${term.cols}x${term.rows}`;
      if (size === lastSize) return;
      lastSize = size;
      transport?.resize(term.cols, term.rows);
    };
    const sendInput = (data: string) => transport?.input(data);
    const input = (seq: string) => {
      const m = live.current.modifier;
      const out = applyModifier(m, seq);
      sendInput(out);
      if (m && out !== seq) live.current.onModifierUsed(); // armed until a key it can apply to
    };

    // Cell size in px, measured from the renderer once it has drawn.
    const cellSize = () => {
      const core = (term as unknown as { _core?: { _renderService?: { dimensions?: { css?: { cell?: { width: number; height: number } } } } } })._core;
      const c = core?._renderService?.dimensions?.css?.cell;
      const size = term.options.fontSize!;
      return { w: c?.width || size * 0.6, h: c?.height || size * 1.2 };
    };

    // applyGeometry sizes the grid: fit → to the mount; keep → the pane's own
    // grid, with the font shrunk until the width fits (down to MIN_FONT, then
    // the mount scrolls). A minimum width in fit mode makes the grid wider
    // than the screen when the screen is narrower — the mount then scrolls
    // sideways, which is how a TUI without horizontal scrolling (a diff
    // viewer, a table) still shows its right-hand side.
    const PAD = 8; // the mount's padding, both sides
    const applyGeometry = () => {
      if (mount.clientWidth === 0) return; // hidden: nothing to measure, sized again when shown
      const g = live.current;
      mount.classList.remove("keep", "wide");
      if (g.mode === "keep" && g.cols > 0 && g.rows > 0) {
        const avail = mount.clientWidth - PAD;
        const size = Math.max(MIN_FONT, Math.min(g.fontSize, Math.floor((avail * term.options.fontSize!) / (g.cols * cellSize().w))));
        if (term.options.fontSize !== size) term.options.fontSize = size;
        if (term.cols !== g.cols || term.rows !== g.rows) term.resize(g.cols, g.rows);
        mount.classList.add("keep");
      } else {
        if (term.options.fontSize !== g.fontSize) term.options.fontSize = g.fontSize;
        const fits = fit.proposeDimensions();
        if (g.minCols > 0 && fits && fits.cols < g.minCols) {
          // the class brings the scrollbar first, so clientHeight already
          // excludes it when the rows are measured — one resize, not two
          mount.classList.add("wide");
          const rows = Math.max(5, Math.floor((mount.clientHeight - PAD) / cellSize().h));
          if (term.cols !== g.minCols || term.rows !== rows) term.resize(g.minCols, rows);
        } else {
          fit.fit();
        }
      }
      sendResize();
    };
    applyRef.current = applyGeometry;
    focusRef.current = () => term.focus();
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

    // Output from either transport: the mixed DECSETs split (see
    // splitTracking), then the follow-up work once it is on screen. One
    // decoder for the stream, so a character cut by a chunk boundary joins.
    const decoder = new TextDecoder();
    let onPainted = () => {}; // the content-anchored selection, below
    const paint = (buf: Uint8Array) => {
      const text = decoder.decode(buf, { stream: true });
      term.write(splitTracking(text), () => {
        followBottom();
        scheduleLayer();
        onPainted();
      });
    };

    // A fresh stream starts from a blank screen (there is no replay; the
    // server nudges the app into a repaint). The reset forgets the modes
    // the app set, and nobody will set them again: what was known is
    // re-applied, and the paste rule falls back to "bracket when unsure".
    const onOpen = (sentSize: string) => {
      const appCursor = term.modes.applicationCursorKeysMode;
      const bracketed = term.modes.bracketedPasteMode;
      decoder.decode(); // a character cut by the old stream's end is dropped
      term.reset();
      forgetSelection();
      pasteModeSeen = false;
      if (appCursor) term.write("\x1b[?1h");
      if (bracketed) term.write("\x1b[?2004h");
      lastSize = sentSize; // what the connect request carried
      sendResize(); // only when the grid changed since then
    };

    // wsWouldAccept tells a proxy that drops WebSockets from everything
    // else that ends a socket before it opens — a server restarting, a pane
    // gone (404), a dead remote (503). The same URL as a plain request
    // passes the handler's checks and fails only at the upgrade itself (426
    // Upgrade Required): then, and only then, the proxy is the problem.
    const wsWouldAccept = () => fetch(`/ws/pane?${paneQuery}`).then((r) => r.status === 426, () => false);

    const connectWS = (): Transport => {
      const proto = location.protocol === "https:" ? "wss" : "ws";
      const sentSize = `${term.cols}x${term.rows}`;
      let ws: WebSocket | null = new WebSocket(`${proto}://${location.host}/ws/pane?${paneQuery}&cols=${term.cols}&rows=${term.rows}`);
      ws.binaryType = "arraybuffer";
      let opened = false;
      let retryTimer: ReturnType<typeof setTimeout> | null = null;
      const send = (obj: object) => {
        if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(obj));
      };
      const retry = () => {
        retryTimer = setTimeout(() => {
          if (!closed) transport = connectWS();
        }, Math.min(5000, 250 * 2 ** wsRetry++));
      };
      ws.onopen = () => {
        opened = true;
        wsFailures = 0;
        onOpen(sentSize);
      };
      ws.onmessage = (ev) => {
        if (!(ev.data instanceof ArrayBuffer)) return;
        wsRetry = 0; // output flowing: the back-off starts over
        paint(new Uint8Array(ev.data));
      };
      ws.onerror = () => ws?.close();
      ws.onclose = (ev) => {
        if (closed || ws === null) return;
        if (opened) {
          // a normal close means the attach ended — herdr restarted, the
          // SSH session blinked, the pane went away. The next connect
          // starts a fresh attach; a pane that is gone fails that connect
          // and the roster drops the terminal soon after. A reason (an
          // attach refused: another client holds the terminal) is written
          // on the screen — every retry starts from a blank one, so it is
          // written each time and the screen always shows it
          if (ev.code === 1000) note("terminal detached — reconnecting");
          else if (ev.reason) note(ev.reason);
          retry();
          return;
        }
        void wsWouldAccept().then((would) => {
          if (closed) return;
          if (would && ++wsFailures >= 2) {
            preferHTTP = true;
            note("WebSocket unavailable here — using the HTTP transport");
            transport = connectHTTP();
            return;
          }
          retry();
        });
      };
      return {
        input: (data) => send({ type: "input", data }),
        resize: (cols, rows) => send({ type: "resize", cols, rows }),
        close: () => {
          if (retryTimer) clearTimeout(retryTimer);
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
      let reopenTimer: ReturnType<typeof setTimeout> | null = null;

      const closeStream = () => {
        if (!sid) return;
        void fetch(`/api/term/close?sid=${sid}`, { method: "DELETE", keepalive: true });
        sid = null;
      };
      const open = async () => {
        const sentSize = `${term.cols}x${term.rows}`;
        const res = await fetch(`/api/term/open?${paneQuery}&cols=${term.cols}&rows=${term.rows}`, { method: "POST" });
        if (!res.ok) throw new Error(`open failed (${res.status})`);
        sid = ((await res.json()) as { sid: string }).sid;
        if (stopped) {
          closeStream(); // closed while opening: nothing may pin the attach
          return;
        }
        onOpen(sentSize);
      };
      // input goes in small batches; a batch that failed is queued again
      // and retried, so nothing waits for the next keystroke
      const flush = async () => {
        flushTimer = null;
        if (stopped || !sid || !pending) return;
        const body = pending;
        pending = "";
        try {
          await fetch(`/api/term/input?sid=${sid}`, { method: "POST", body });
        } catch {
          pending = body + pending;
          if (!stopped && !flushTimer) flushTimer = setTimeout(() => void flush(), 500);
        }
      };
      const later = (ms: number) => {
        if (!stopped) reopenTimer = setTimeout(() => void loop(), ms);
      };
      const loop = async () => {
        reopenTimer = null;
        if (stopped) return;
        try {
          await open();
        } catch (e) {
          if (!stopped) {
            note(`cannot open the terminal stream (${e instanceof Error ? e.message : String(e)}) — retrying`);
            later(2000);
          }
          return;
        }
        while (!stopped && sid) {
          try {
            const res = await fetch(`/api/term/read?sid=${sid}&wait=20`);
            if (stopped) return;
            if (res.status === 410) {
              sid = null;
              later(500); // expired: reopen
              return;
            }
            if (!res.ok) throw new Error(`read failed (${res.status})`);
            const buf = new Uint8Array(await res.arrayBuffer());
            if (stopped) return;
            if (buf.length) paint(buf);
            if (res.headers.get("X-Remuda-End") === "1") {
              sid = null; // the server dropped it already
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
          if (reopenTimer) clearTimeout(reopenTimer);
          if (flushTimer) clearTimeout(flushTimer);
          closeStream();
        },
      };
    };

    const onData = term.onData((data) => {
      forgetSelection(); // typing ends a selection, as in any terminal
      input(data);
    });

    // ---- the mouse stays ours ----
    // herdr enables mouse tracking in the attach stream (for its own
    // scrollback). If xterm honoured that, every drag would become mouse
    // reports and nothing could ever be selected. So the DECSET for the
    // tracking modes is intercepted here — xterm never enters tracking, a
    // plain drag selects — and the wheel is translated into the reports the
    // app asked for, so herdr still scrolls.
    let appMouse = 0; // the tracking mode the app wants; 0 = none
    let sgrMouse = false; // it asked for SGR (1006) reports
    // Whether the app ever said how it wants pastes (DECSET 2004). A viewer
    // that attaches later gets no replay, so it never sees Claude Code's
    // "bracketed paste on" from startup — and xterm would then paste a
    // multi-line text raw, every newline an Enter that sends the prompt.
    let pasteModeSeen = false;
    const mouseModeHandler = (enable: boolean) => (params: (number | number[])[]) => {
      const rest: number[] = [];
      let touched = false;
      for (const p of params) {
        const n = Array.isArray(p) ? p[0]! : p;
        if (TRACKING.has(n)) {
          touched = true;
          appMouse = enable ? n : 0;
        } else {
          if (n === 1006) sgrMouse = enable;
          if (n === 2004) pasteModeSeen = true;
          rest.push(n);
        }
      }
      if (!touched) return false; // nothing of ours: xterm handles it
      // a mixed sequence only gets here when a chunk boundary cut it before
      // splitTracking could; the other modes are re-applied, late
      if (rest.length) term.write(`\x1b[?${rest.join(";")}${enable ? "h" : "l"}`);
      // xterm only swaps its text cursor for an arrow when it tracks the
      // mouse itself; the app owns the clicks, so the arrow is ours to show
      mount.classList.toggle("app-mouse", appMouse !== 0);
      return true;
    };
    const decset = term.parser.registerCsiHandler({ prefix: "?", final: "h" }, mouseModeHandler(true));
    const decrst = term.parser.registerCsiHandler({ prefix: "?", final: "l" }, mouseModeHandler(false));

    // the grid cell under a pointer event, 1-based
    const cellAt = (e: MouseEvent): { col: number; row: number } | null => {
      const screen = term.element?.querySelector(".xterm-screen") as HTMLElement | null;
      if (!screen) return null;
      const r = screen.getBoundingClientRect();
      const { w, h } = cellSize();
      return {
        col: Math.max(1, Math.min(term.cols, Math.floor((e.clientX - r.left) / w) + 1)),
        row: Math.max(1, Math.min(term.rows, Math.floor((e.clientY - r.top) / h) + 1)),
      };
    };
    // one mouse report the way the app asked for it: SGR, else X10 (whose
    // release is button 3 and whose coordinates stop at 223)
    const report = (code: number, c: { col: number; row: number }, release: boolean) =>
      sgrMouse
        ? `\x1b[<${code};${c.col};${c.row}${release ? "m" : "M"}`
        : "\x1b[M" + String.fromCharCode(32 + (release ? (code & ~3) | 3 : code), 32 + Math.min(c.col, 223), 32 + Math.min(c.row, 223));
    const modifierBits = (e: MouseEvent) => (e.shiftKey ? 4 : 0) + (e.altKey ? 8 : 0) + (e.ctrlKey ? 16 : 0);

    // A wheel report per row of movement, the way a native terminal does
    // it: the pixels add up and every cell height sends one tick. A
    // trackpad's stream of small events (and its momentum) then scrolls
    // the app as far as the finger moved, not once per event.
    let wheelAcc = { x: 0, y: 0 };
    const wheelTicks = (e: WheelEvent, sideways: boolean): number => {
      const { w, h } = cellSize();
      let delta = sideways ? e.deltaX : e.deltaY;
      if (e.deltaMode === 1) delta *= h; // lines
      else if (e.deltaMode === 2) delta *= h * term.rows; // pages
      const axis = sideways ? "x" : "y";
      if (Math.sign(delta) !== Math.sign(wheelAcc[axis])) wheelAcc[axis] = 0; // a turn forgets the remainder
      wheelAcc[axis] += delta;
      const unit = sideways ? w * 2 : h;
      const ticks = Math.trunc(wheelAcc[axis] / unit);
      wheelAcc[axis] -= ticks * unit;
      return ticks;
    };
    const scrollReport = (ticks: number, sideways: boolean, cell: { col: number; row: number }, mods = 0) => {
      if (!ticks) return;
      const code = sideways ? (ticks < 0 ? 66 : 67) : ticks < 0 ? 64 : 65;
      sendInput(report(code + mods, cell, false).repeat(Math.min(40, Math.abs(ticks))));
    };
    const onWheel = (e: WheelEvent) => {
      const sideways = Math.abs(e.deltaX) > Math.abs(e.deltaY);
      // a grid larger than the screen (keep mode, a minimum width) scrolls
      // here, in that direction; otherwise the gesture is the app's —
      // wheel left/right (buttons 6/7) is how a TUI scrolls its columns
      if (sideways ? mount.scrollWidth > mount.clientWidth : mount.classList.contains("keep") && mount.scrollHeight > mount.clientHeight) {
        e.stopPropagation();
        return;
      }
      if (!appMouse) return; // no tracking wanted: xterm scrolls its own buffer
      const cell = cellAt(e);
      if (!cell) return;
      e.preventDefault();
      e.stopPropagation();
      if (pressed) pointer = { x: e.clientX, y: e.clientY }; // a wheel under a held button
      scrollReport(wheelTicks(e, sideways), sideways, cell, modifierBits(e));
    };
    mount.addEventListener("wheel", onWheel, { capture: true, passive: false });

    // A finger does the same: a drag over the screen is the app's scroll,
    // one tick per row moved, so a phone can read back through an agent's
    // output. Without this xterm would scroll its own (empty) buffer and
    // the app would never know. A tap stays a tap: the browser's click
    // follows and is reported below. Selection mode keeps the finger.
    let touch: { y: number; x: number; acc: number } | null = null;
    const onTouchStart = (e: TouchEvent) => {
      touch = null;
      if (!appMouse || !layer.hidden || e.touches.length !== 1) return;
      const t = e.touches[0]!;
      touch = { x: t.clientX, y: t.clientY, acc: 0 };
    };
    const onTouchMove = (e: TouchEvent) => {
      if (!touch || e.touches.length !== 1) return;
      const t = e.touches[0]!;
      const dy = touch.y - t.clientY; // the finger goes up: the text follows it, the view scrolls down
      touch.y = t.clientY;
      touch.acc += dy;
      const h = cellSize().h;
      const ticks = Math.trunc(touch.acc / h);
      touch.acc -= ticks * h;
      e.preventDefault();
      e.stopPropagation();
      const cell = cellAt({ clientX: t.clientX, clientY: t.clientY } as MouseEvent);
      if (cell) scrollReport(ticks, false, cell);
    };
    const onTouchEnd = () => {
      touch = null;
    };
    mount.addEventListener("touchstart", onTouchStart, { capture: true, passive: true });
    mount.addEventListener("touchmove", onTouchMove, { capture: true, passive: false });
    mount.addEventListener("touchend", onTouchEnd, { capture: true, passive: true });
    mount.addEventListener("touchcancel", onTouchEnd, { capture: true, passive: true });

    // Clicks still reach the app: a press and release on the same spot is
    // reported at its cell (a TUI's buttons and lists work), while anything
    // that moved is a selection and stays here. Reported after xterm's own
    // mouseup, so a click that opened a link is not also a click for the app.
    let pressed: { x: number; y: number; button: number } | null = null;
    let linkUsed = false;
    const onMouseDown = (e: MouseEvent) => {
      pressed = { x: e.clientX, y: e.clientY, button: e.button };
      linkUsed = false;
      if (e.button === 0 && !e.shiftKey && !layer.contains(e.target as Node)) dragStart(e);
    };
    const onMouseUp = (e: MouseEvent) => {
      const p = pressed;
      pressed = null;
      dragEnd();
      if (!p || !appMouse || layer.contains(e.target as Node)) return;
      if (p.button !== e.button || Math.hypot(e.clientX - p.x, e.clientY - p.y) > 4) return;
      const cell = cellAt(e);
      if (!cell) return;
      const code = e.button + modifierBits(e);
      setTimeout(() => {
        if (!linkUsed) sendInput(report(code, cell, false) + report(code, cell, true));
      }, 0);
    };
    mount.addEventListener("mousedown", onMouseDown, true);
    mount.addEventListener("mouseup", onMouseUp, true);

    // ---- a selection that stays with the text ----
    // The app owns the scrollback: a wheel is a report to it and it repaints
    // the screen, so a selection on screen rows would sit still while the
    // text moved under it. Here the selection is kept in content rows, and
    // every row seen while it exists is remembered by its text. When the
    // screen settles after a repaint, the rows on it are looked up in that
    // memory: the offset most unique rows agree on is where the screen is
    // now, the selection is drawn there (or not at all, when the screen
    // shows nothing remembered — a jump far away), and a drag held past the
    // top or bottom edge scrolls the app and grows the selection. Copy
    // reads the remembered rows, so it is right wherever the screen went.
    type Pos = { row: number; col: number }; // content row, 0-based col
    let offset = 0; // content row = screen row + offset
    let attached = true; // the screen's place in the content is known
    let anchor: Pos | null = null; // where the selection began
    let head: Pos | null = null; // where it is being dragged to
    const rowText = new Map<number, string>(); // content row → its text when last seen
    let applying = false; // our own term.select, not the user's
    let settleTimer: ReturnType<typeof setTimeout> | null = null;
    const screenRows = (): string[] => {
      const buf = term.buffer.active;
      const out: string[] = [];
      for (let y = 0; y < term.rows; y++) out.push(buf.getLine(buf.baseY + y)?.translateToString(false) ?? "");
      return out;
    };
    const remember = (rows: string[]) => {
      for (let y = 0; y < rows.length; y++) rowText.set(y + offset, rows[y]!);
    };
    const before = (a: Pos, b: Pos) => a.row < b.row || (a.row === b.row && a.col < b.col);
    const ordered = (): [Pos, Pos] | null => (anchor && head ? (before(anchor, head) ? [anchor, head] : [head, anchor]) : null);
    const forgetSelection = () => {
      anchor = head = null;
      offset = 0;
      attached = true;
      rowText.clear();
    };
    // where the screen is in the content: the offset that the most rows
    // agree on, among rows whose text is unique both on screen and in the
    // memory (a repeated line fits anywhere). null when nothing is known.
    const relocate = (rows: string[]): number | null => {
      const onScreen = new Map<string, number>();
      const twice = new Set<string>();
      rows.forEach((r, y) => {
        if (r.trim().length < 4) return;
        if (onScreen.has(r)) twice.add(r);
        else onScreen.set(r, y);
      });
      const inMemory = new Map<string, number>();
      for (const [row, text] of rowText) {
        if (!onScreen.has(text) || twice.has(text)) continue;
        if (inMemory.has(text)) twice.add(text);
        else inMemory.set(text, row);
      }
      const votes = new Map<number, number>();
      let total = 0;
      for (const [text, row] of inMemory) {
        if (twice.has(text)) continue;
        const o = row - onScreen.get(text)!;
        votes.set(o, (votes.get(o) ?? 0) + 1);
        total++;
      }
      let best: number | null = null;
      let bestVotes = 0;
      for (const [o, v] of votes) {
        if (v > bestVotes || (v === bestVotes && o === offset)) [best, bestVotes] = [o, v];
      }
      // a few rows agreeing is not enough: the app's own frame — a header
      // row, the status lines — sits on the same screen rows whatever the
      // content, and after a jump into unseen text those would be the only
      // matches, all voting for the old place. A good share of the screen
      // has to be recognised.
      let unique = 0;
      for (const r of onScreen.keys()) if (!twice.has(r)) unique++;
      return best !== null && bestVotes >= 2 && bestVotes * 2 > total && bestVotes >= unique * 0.3 ? best : null;
    };
    // draw the part of the selection that is on screen
    const renderSelection = () => {
      const o = ordered();
      if (!o) return;
      const [s0, e0] = o;
      const top = offset;
      const bottom = offset + term.rows - 1;
      applying = true;
      try {
        if (!attached || e0.row < top || s0.row > bottom) {
          if (term.hasSelection()) term.clearSelection();
          return;
        }
        const s = s0.row < top ? { row: top, col: 0 } : s0;
        const e = e0.row > bottom ? { row: bottom, col: term.cols } : e0;
        const length = (e.row - s.row) * term.cols + (e.col - s.col);
        if (length > 0) term.select(s.col, term.buffer.active.baseY + s.row - offset, length);
        else if (term.hasSelection()) term.clearSelection();
      } finally {
        applying = false;
      }
    };
    // a settled screen: found in the memory (then remembered and the
    // selection drawn), or not (the selection waits, unseen)
    const settled = () => {
      settleTimer = null;
      if (!anchor) return;
      const rows = screenRows();
      const found = relocate(rows);
      if (found === null) {
        attached = false;
      } else {
        const moved = found !== offset;
        offset = found;
        attached = true;
        remember(rows);
        if (edge === "down" && head) head = { row: offset + term.rows - 1, col: term.cols };
        else if (edge === "up" && head) head = { row: offset, col: 0 };
        else if (moved && dragAnchor && pointer) {
          // the button is held and the text scrolled under the pointer (a
          // wheel): the selection runs from where it began to what is now
          // under the pointer, as in a native terminal
          head = pointerPos() ?? head;
        }
      }
      renderSelection();
    };
    onPainted = () => {
      if (!anchor) return;
      if (settleTimer) clearTimeout(settleTimer);
      settleTimer = setTimeout(settled, 40);
    };
    // the user's own selection changes: a drag, a double-click, a shift-click
    const onSelection = term.onSelectionChange(() => {
      if (applying) return;
      const pos = term.getSelectionPosition();
      // xterm drops its own selection on any user input and on a buffer
      // switch; ours stays until a new drag (mousedown) or typing says so
      if (!pos) return;
      const baseY = term.buffer.active.baseY;
      const start: Pos = { row: pos.start.y - baseY + offset, col: pos.start.x };
      const end: Pos = { row: pos.end.y - baseY + offset, col: pos.end.x };
      if (!anchor) remember(screenRows());
      // an end clipped at the screen's edge keeps the rows beyond it
      const o = ordered();
      const keepStart = o && o[0].row < offset && start.row === offset && start.col === 0;
      const keepEnd = o && o[1].row > offset + term.rows - 1 && end.row === offset + term.rows - 1;
      const s = keepStart ? o![0] : start;
      const e = keepEnd ? o![1] : end;
      if (dragAnchor && dragMoved) {
        anchor = dragAnchor;
        head = before(anchor, s) || (anchor.row === s.row && anchor.col === s.col) ? e : s;
      } else {
        anchor = s; // a word or a line (double, triple click), a shift-click
        head = e;
      }
    });
    // the drag: its anchor, and the scroll when it leaves the screen
    // A drag is ours from its first move: xterm reports a selection only on
    // mouseup, so a scroll under a held button would go unseen until then
    // and land on the wrong rows. The anchor is set on mousedown, the head
    // follows the pointer, and the first select() ends xterm's own drag.
    // A click that never moves is left to xterm (a double-click's word, a
    // triple-click's line arrive through onSelectionChange).
    let dragAnchor: Pos | null = null;
    let dragMoved = false;
    let pointer: { x: number; y: number } | null = null; // where the held button is
    const pointerPos = (): Pos | null => {
      if (!pointer) return null;
      const cell = cellAt({ clientX: pointer.x, clientY: pointer.y } as MouseEvent);
      return cell ? { row: cell.row - 1 + offset, col: cell.col - 1 } : null;
    };
    let edge: "up" | "down" | null = null;
    let edgeTimer: ReturnType<typeof setInterval> | null = null;
    const edgeTick = () => {
      if (!edge || !appMouse) return;
      const screen = term.element?.querySelector(".xterm-screen") as HTMLElement | null;
      if (!screen) return;
      const r = screen.getBoundingClientRect();
      const cell = cellAt({ clientX: r.left + 1, clientY: edge === "down" ? r.bottom - 1 : r.top + 1 } as MouseEvent);
      if (cell) scrollReport(edge === "down" ? 1 : -1, false, cell);
    };
    const onDragMove = (e: MouseEvent) => {
      pointer = { x: e.clientX, y: e.clientY };
      dragMoved = true;
      if (!edge) {
        head = pointerPos() ?? head;
        renderSelection();
      }
      const screen = term.element?.querySelector(".xterm-screen") as HTMLElement | null;
      if (!screen) return;
      const r = screen.getBoundingClientRect();
      const next: typeof edge = e.clientY > r.bottom ? "down" : e.clientY < r.top ? "up" : null;
      if (next === edge) return;
      edge = next;
      if (edgeTimer) clearInterval(edgeTimer);
      edgeTimer = edge ? setInterval(edgeTick, 90) : null;
    };
    const dragStart = (e: MouseEvent) => {
      forgetSelection(); // a new selection starts over, with a fresh memory
      const cell = cellAt(e);
      dragAnchor = cell ? { row: cell.row - 1, col: cell.col - 1 } : null;
      dragMoved = false;
      pointer = { x: e.clientX, y: e.clientY };
      if (dragAnchor) {
        anchor = head = dragAnchor;
        remember(screenRows());
      }
      document.addEventListener("mousemove", onDragMove);
    };
    const dragEnd = () => {
      if (!dragMoved) forgetSelection(); // a click; xterm's word or line may still follow
      dragAnchor = null;
      pointer = null;
      edge = null;
      if (edgeTimer) clearInterval(edgeTimer);
      edgeTimer = null;
      document.removeEventListener("mousemove", onDragMove);
    };
    // the selected text, on screen or remembered
    const contentSelection = (): string => {
      const o = ordered();
      if (!o) return "";
      const [s, e] = o;
      const lines: string[] = [];
      for (let r = s.row; r <= e.row; r++) {
        let t = rowText.get(r) ?? "";
        if (r === e.row) t = t.slice(0, e.col);
        if (r === s.row) t = t.slice(s.col);
        lines.push(t.replace(/[ \t\u00a0]+$/, ""));
      }
      return lines.join("\n");
    };

    // ---- selection and copy ----
    // Two ways to select: a plain drag (xterm's own — the app's mouse
    // tracking is intercepted above, so a drag is never a mouse report), and
    // selection mode — a transparent text layer over the grid whose
    // selection is the browser's own, for a phone's long-press and the
    // places where xterm's does not work. Copy is Cmd+C / Ctrl+Shift+C, or
    // the native menu.
    const layer = document.createElement("div");
    layer.className = "term-select";
    layer.hidden = true;
    term.element!.appendChild(layer);
    let layerTimer: ReturnType<typeof setTimeout> | null = null;

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
      layer.style.lineHeight = `${cellSize().h}px`;
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

    // what is selected right now: the layer's DOM selection, else xterm's.
    // A full-screen app paints its rows out to the last column with real
    // spaces, so a selection would carry every line padded to the width
    // of the screen; the trailing run goes, as a native terminal drops it
    const selectedText = (): string => {
      const sel = window.getSelection();
      const raw = sel && !sel.isCollapsed && layer.contains(sel.anchorNode) ? sel.toString() : anchor ? contentSelection() : term.hasSelection() ? term.getSelection() : "";
      return raw
        .split("\n")
        .map((l) => l.replace(/[ \t\u00a0]+$/, ""))
        .join("\n");
    };
    const copySelection = (): boolean => {
      const text = selectedText();
      if (!text) return false;
      void copyText(text).then((ok) => ok && live.current.onCopied(text.length));
      return true;
    };
    // the native copy (the context menu, a phone's Copy): the selection may
    // live in the layer, so the text is set on the event ourselves
    const onCopy = (e: ClipboardEvent) => {
      const text = selectedText();
      if (!text) return;
      e.clipboardData?.setData("text/plain", text);
      e.preventDefault();
      live.current.onCopied(text.length);
    };
    mount.addEventListener("copy", onCopy);

    // Keys xterm gets wrong for agent TUIs: Shift+Enter is a newline in
    // Claude Code only as the Alt+Enter chord (ESC CR); where Ctrl+V is the
    // paste chord it must reach the browser's paste event instead of
    // becoming 0x16 (on a Mac it is a keystroke for the app). The handler
    // sees keydown, keypress AND keyup — Enter's keypress would still emit
    // "\r" and send the prompt, so every phase of the chord is swallowed.
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

    // File paths the agent prints become links: a click opens the file where
    // it lives (this machine or the remote one), at the :line if given.
    const linkProvider = term.registerLinkProvider({
      provideLinks(y, callback) {
        const row = term.buffer.active.getLine(y - 1);
        if (!row) return callback(undefined);
        const text = row.translateToString(true);
        const decorations = { pointerCursor: true, underline: true };
        const links: ILink[] = findUrls(text).map((m) => ({
          range: { start: { x: m.start + 1, y }, end: { x: m.end, y } },
          text: m.text,
          decorations,
          activate: (e: MouseEvent) => {
            if (!live.current.agent && !e.metaKey && !e.ctrlKey) return; // the app's click
            linkUsed = true;
            openUrl(m.url);
          },
        }));
        if (live.current.agent) {
          for (const m of findPaths(text)) {
            links.push({
              range: { start: { x: m.start + 1, y }, end: { x: m.end, y } },
              text: m.text,
              decorations,
              activate: () => {
                linkUsed = true;
                live.current.onOpenFile({ path: m.path, line: m.line });
              },
            });
          }
        }
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
      if (files.length > 0) {
        e.preventDefault();
        e.stopPropagation();
        typeFiles(files);
        return;
      }
      // Plain text: xterm's own paste, which brackets it when the app asked.
      // When nobody knows what the app asked (a late attach), a text with
      // line breaks is bracketed anyway — every shell and agent TUI reads
      // that as one paste; the alternative sends line one and loses the rest.
      const text = e.clipboardData?.getData("text/plain") ?? "";
      if (!text.includes("\n") || term.modes.bracketedPasteMode || pasteModeSeen) return;
      e.preventDefault();
      e.stopPropagation();
      sendInput("\x1b[200~" + text.replace(/\r\n/g, "\r").replace(/\n/g, "\r") + "\x1b[201~");
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
      onSelection.dispose();
      dragEnd();
      if (settleTimer) clearTimeout(settleTimer);
      decset.dispose();
      decrst.dispose();
      mount.removeEventListener("wheel", onWheel, { capture: true });
      mount.removeEventListener("touchstart", onTouchStart, { capture: true });
      mount.removeEventListener("touchmove", onTouchMove, { capture: true });
      mount.removeEventListener("touchend", onTouchEnd, { capture: true });
      mount.removeEventListener("touchcancel", onTouchEnd, { capture: true });
      mount.removeEventListener("mousedown", onMouseDown, true);
      mount.removeEventListener("mouseup", onMouseUp, true);
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
  }, [keepCols, keepRows, mode, minCols, fontSize]);

  useEffect(() => {
    selectRef.current(selectMode);
  }, [selectMode]);
  // back on screen (not the first show): the grid is measured again and,
  // with a keyboard, the keys go here — a touch screen's keyboard must not pop
  const wasActive = useRef(active);
  useEffect(() => {
    const was = wasActive.current;
    wasActive.current = active;
    if (!active || was) return;
    applyRef.current();
    if (!isTouch) focusRef.current();
  }, [active]);

  return <div ref={mountRef} className="term-mount" />;
}
