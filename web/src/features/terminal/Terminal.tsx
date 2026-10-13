import { useEffect, useRef } from "react";
import { Terminal as XTerm, type ILink } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import "@xterm/xterm/css/xterm.css";
import { uploadFiles } from "@/features/files/api";
import { filesFrom } from "@/lib/transfer";
import { shellQuote } from "@/lib/text";
import { copyText, openUrl } from "@/lib/host";
import { isTouch } from "@/lib/prefs";
import { findPaths, findUrls } from "@/lib/paths";
import type { FileTarget } from "@/features/files/target";

import type { Modifier, SizeMode, TermApi } from "./engine/types";

const MIN_FONT = 8;
const MAC = /Mac|iPhone|iPad/.test(navigator.platform);

// Once WebSockets prove unusable, every terminal on this page uses HTTP without trying again.
let preferHTTP = false;
let wsFailures = 0;
let wsRetry = 0;

// applyModifier encodes a printable character or an arrow under a one-shot Ctrl or Alt (an arrow
// as the modified CSI form, ESC [ 1 ; 5 A for Ctrl, ; 3 for Alt); anything else passes through.
export function applyModifier(mod: Modifier, seq: string): string {
  if (!mod) return seq;
  const arrow = /^\x1b[[O]([ABCD])$/.exec(seq);
  if (arrow) return `\x1b[1;${mod === "ctrl" ? 5 : 3}${arrow[1]}`;
  if (seq.length !== 1 || seq < " " || seq > "~") return seq;
  return mod === "ctrl" ? String.fromCharCode(seq.toUpperCase().charCodeAt(0) & 0x1f) : "\x1b" + seq;
}

const TRACKING = new Set([9, 1000, 1002, 1003]);
const DECSET = /\x1b\[\?([\d;]+)([hl])/g;
// splitTracking splits a DECSET that mixes a mouse-tracking mode with other modes in two, the others
// first: a sequence re-injected from xterm's CSI handler would land behind the rest of the chunk.
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

// TerminalView is the live terminal for one herdr pane: pane bytes arrive as binary WS frames,
// keystrokes go back as JSON text frames; herdr keeps the real scrollback.
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
  // an agent's pane: file paths and URLs are links; in a shell's pane only a Cmd/Ctrl-click opens a URL
  agent: boolean;
  mode: SizeMode;
  minCols: number; // fit mode: never narrower than this many columns (0 = off)
  fontSize: number; // preferred size in fit mode
  modifier: Modifier;
  // a transparent text layer over the grid whose selection is the browser's own (a phone's long-press)
  selectMode: boolean;
  onModifierUsed: () => void;
  onReady: (api: TermApi | null) => void;
  onOpenFile: (t: FileTarget) => void;
  onCopied: (chars: number) => void;
  active?: boolean; // the lens on screen; hidden, the attach stays and output keeps arriving
}) {
  const mountRef = useRef<HTMLDivElement>(null);
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
    type Transport = { input(data: string): void; resize(cols: number, rows: number): void; close(): void };
    let transport: Transport | null = null;
    const paneQuery = `machine=${encodeURIComponent(machineId)}&id=${encodeURIComponent(paneId)}`;
    const note = (text: string) => term.write(`\r\n\x1b[2m[remuda] ${text}\x1b[0m\r\n`);

    // sendResize sends the grid to the shared PTY only when it changed since the connect request carried it.
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
      if (m && out !== seq) live.current.onModifierUsed();
    };

    // cellSize reads the cell box in px from xterm's private _renderService; before the first draw, an estimate.
    const cellSize = () => {
      const core = (term as unknown as { _core?: { _renderService?: { dimensions?: { css?: { cell?: { width: number; height: number } } } } } })._core;
      const c = core?._renderService?.dimensions?.css?.cell;
      const size = term.options.fontSize!;
      return { w: c?.width || size * 0.6, h: c?.height || size * 1.2 };
    };

    // applyGeometry sizes the grid: fit → to the mount (at least minCols wide, the mount scrolling
    // sideways), keep → the pane's own grid with the font shrunk to fit the width, down to MIN_FONT.
    const PAD = 8; // the mount's padding, both sides
    const applyGeometry = () => {
      if (mount.clientWidth === 0) return; // hidden: nothing to measure
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
          // the class brings the scrollbar first, so clientHeight already excludes it when the rows are measured
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

    // followBottom keeps a keep-mode grid taller than the screen at its bottom unless the reader scrolled up.
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

    // paint writes a chunk from either transport; one streaming decoder, so a character cut by a chunk boundary joins.
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

    // onOpen resets the screen for a fresh stream: there is no replay, the server nudges the app into a
    // repaint, so the modes xterm knew are re-applied by hand since nobody will set them again.
    const onOpen = (sentSize: string) => {
      const appCursor = term.modes.applicationCursorKeysMode;
      const bracketed = term.modes.bracketedPasteMode;
      decoder.decode(); // a character cut by the old stream's end is dropped
      term.reset();
      forgetSelection();
      pasteModeSeen = false;
      if (appCursor) term.write("\x1b[?1h");
      if (bracketed) term.write("\x1b[?2004h");
      lastSize = sentSize;
      sendResize();
    };

    // wsWouldAccept tells a proxy that drops WebSockets from a server that ends the socket itself: a plain
    // GET of the same URL passes every check and fails only at the upgrade (426), so 426 means the proxy.
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
        wsRetry = 0;
        paint(new Uint8Array(ev.data));
      };
      ws.onerror = () => ws?.close();
      ws.onclose = (ev) => {
        if (closed || ws === null) return;
        if (opened) {
          // a close reason (an attach refused: another client holds the terminal) is written on every
          // retry, since each one starts from a blank screen
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

    // connectHTTP opens a server-side stream, long-polls it for output and POSTs input in batches; 410 means it expired.
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
      // flush POSTs the pending input; a batch that failed is queued again and retried.
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
              later(500);
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
      forgetSelection();
      input(data);
    });

    // ---- the mouse stays ours ----
    // herdr enables mouse tracking in every attach stream; honoured by xterm, every drag would be mouse
    // reports. The tracking DECSETs are intercepted so xterm never tracks, and the wheel is reported by hand.
    let appMouse = 0; // the tracking mode the app wants; 0 = none
    let sgrMouse = false; // it asked for SGR (1006) reports
    // whether a DECSET 2004 ever passed: a late attach gets no replay, so Claude Code's "bracketed paste on" is never seen
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
      // a mixed sequence only gets here when a chunk boundary cut it before splitTracking could: the rest is re-applied, late
      if (rest.length) term.write(`\x1b[?${rest.join(";")}${enable ? "h" : "l"}`);
      // xterm shows an arrow cursor only when it tracks the mouse itself; the app owns the clicks, so the arrow is ours
      mount.classList.toggle("app-mouse", appMouse !== 0);
      return true;
    };
    const decset = term.parser.registerCsiHandler({ prefix: "?", final: "h" }, mouseModeHandler(true));
    const decrst = term.parser.registerCsiHandler({ prefix: "?", final: "l" }, mouseModeHandler(false));

    // cellAt is the grid cell under a pointer event, 1-based.
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
    // report encodes one mouse report as the app asked: SGR (1006), else X10, whose release is button 3
    // and whose coordinates stop at 223.
    const report = (code: number, c: { col: number; row: number }, release: boolean) =>
      sgrMouse
        ? `\x1b[<${code};${c.col};${c.row}${release ? "m" : "M"}`
        : "\x1b[M" + String.fromCharCode(32 + (release ? (code & ~3) | 3 : code), 32 + Math.min(c.col, 223), 32 + Math.min(c.row, 223));
    const modifierBits = (e: MouseEvent) => (e.shiftKey ? 4 : 0) + (e.altKey ? 8 : 0) + (e.ctrlKey ? 16 : 0);

    // wheelTicks turns wheel movement into ticks as a native terminal does: the pixels add up, one tick per cell height.
    let wheelAcc = { x: 0, y: 0 };
    const wheelTicks = (e: WheelEvent, sideways: boolean): number => {
      const { w, h } = cellSize();
      let delta = sideways ? e.deltaX : e.deltaY;
      if (e.deltaMode === 1) delta *= h; // lines
      else if (e.deltaMode === 2) delta *= h * term.rows; // pages
      const axis = sideways ? "x" : "y";
      if (Math.sign(delta) !== Math.sign(wheelAcc[axis])) wheelAcc[axis] = 0;
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
      // a grid larger than the screen scrolls here in that direction; otherwise the gesture is the app's (wheel left/right = buttons 6/7)
      if (sideways ? mount.scrollWidth > mount.clientWidth : mount.classList.contains("keep") && mount.scrollHeight > mount.clientHeight) {
        e.stopPropagation();
        return;
      }
      if (!appMouse) return;
      const cell = cellAt(e);
      if (!cell) return;
      e.preventDefault();
      e.stopPropagation();
      if (pressed) pointer = { x: e.clientX, y: e.clientY }; // a wheel under a held button
      scrollReport(wheelTicks(e, sideways), sideways, cell, modifierBits(e));
    };
    mount.addEventListener("wheel", onWheel, { capture: true, passive: false });

    // A finger drag is the app's scroll too, one tick per row moved: otherwise xterm would scroll its
    // own (empty) buffer and the app would never know. A tap stays a tap, reported as a click below.
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

    // A press and release within 4px is reported to the app as a click at its cell; anything that moved
    // is a selection. Reported after xterm's own mouseup, so a click that opened a link is not also the app's.
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
    // The app repaints the screen on every scroll, so the selection is kept in content rows and every row
    // seen is remembered by its text; a settled screen is located in that memory and the selection drawn there.
    type Pos = { row: number; col: number }; // content row, 0-based col
    let offset = 0; // content row = screen row + offset
    let attached = true; // the screen's place in the content is known
    let anchor: Pos | null = null;
    let head: Pos | null = null;
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
    // relocate finds where the screen is in the content: the offset most rows agree on, among rows whose
    // text is unique both on screen and in the memory; null when nothing is known.
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
      // the app's own header and status rows sit on the same screen rows whatever the content and always
      // vote for the old place, so a good share of the screen has to be recognised
      let unique = 0;
      for (const r of onScreen.keys()) if (!twice.has(r)) unique++;
      return best !== null && bestVotes >= 2 && bestVotes * 2 > total && bestVotes >= unique * 0.3 ? best : null;
    };
    // renderSelection draws the part of the selection that is on screen.
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
    // settled locates a settled screen: found in the memory, it is remembered and the selection drawn; else the selection waits unseen.
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
          // a wheel under a held button: the head follows what is now under the pointer, as in a native terminal
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
    // onSelection takes the user's own selection changes: a drag, a double-click, a shift-click.
    const onSelection = term.onSelectionChange(() => {
      if (applying) return;
      const pos = term.getSelectionPosition();
      // xterm drops its own selection on any input and on a buffer switch; ours stays until a new mousedown or typing
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
    // A drag is ours from its first move: xterm reports a selection only on mouseup, so a wheel under a held
    // button would go unseen. A click that never moves is left to xterm (a double-click's word, a triple-click's line).
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
      forgetSelection();
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
    // contentSelection is the selected text, on screen or remembered.
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
    // the selection-mode layer: a transparent text copy of the screen whose selection is the browser's own
    const layer = document.createElement("div");
    layer.className = "term-select";
    layer.hidden = true;
    term.element!.appendChild(layer);
    let layerTimer: ReturnType<typeof setTimeout> | null = null;

    // paintLayer mirrors the visible rows into the layer as text, one row per line.
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

    // selectedText is the layer's DOM selection, else the content selection, else xterm's; a full-screen app
    // pads its rows to the last column with real spaces, so the trailing run goes as a native terminal drops it.
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
    // onCopy serves the native copy (the context menu, a phone's Copy): the selection may live in the layer, so the text is set by hand.
    const onCopy = (e: ClipboardEvent) => {
      const text = selectedText();
      if (!text) return;
      e.clipboardData?.setData("text/plain", text);
      e.preventDefault();
      live.current.onCopied(text.length);
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

    // File paths the agent prints become links: a click opens the file where it lives, at the :line if given.
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
            if (!live.current.agent && !e.metaKey && !e.ctrlKey) return;
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

    // typeFiles uploads dropped or pasted files into the pane's folder and types their paths shell-quoted,
    // never sent; uploads are serialized so two drops don't interleave their paths.
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
      // a late attach never saw DECSET 2004 and xterm would paste a multi-line text raw, every newline an
      // Enter that sends the prompt: it is bracketed by hand instead (CRLF/LF → CR, as xterm does)
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

    // debounced: a window-edge drag would flood the agent with resizes, and Claude Code repaints its whole conversation per resize
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

  // geometry props re-apply without reconnecting; the pane's own grid only matters in keep mode
  const keepCols = mode === "keep" ? cols : 0;
  const keepRows = mode === "keep" ? rows : 0;
  useEffect(() => {
    applyRef.current();
  }, [keepCols, keepRows, mode, minCols, fontSize]);

  useEffect(() => {
    selectRef.current(selectMode);
  }, [selectMode]);
  // back on screen (not the first show): measured again and, with a keyboard, focused — a touch screen's keyboard must not pop
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
