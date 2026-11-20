// The pane's byte stream: a WebSocket, or HTTP long-polling once WebSockets prove unusable on this page
// (a proxy that drops them). Bytes come in, keystrokes and resizes go back; a dropped connection retries.

import { createSender } from "../screen/sender";

export type Transport = {
  input(data: string): void;
  resize(cols: number, rows: number): void;
  close(): void;
};

export type Stream = {
  query: string; // machine=…&id=…
  size: () => { cols: number; rows: number };
  onOpen: (sentSize: string) => void; // a fresh stream, opened with this grid: the screen starts over
  onBytes: (buf: Uint8Array) => void;
  onGrid?: (cols: number, rows: number) => void; // the terminal's grid from here on, in order with the bytes (remuda-core)
  note: (text: string) => void; // a line on the screen from remuda itself, while the stream is down
  inputLost: () => void; // typing did not surely go through, and will not be sent again
  wsOnly?: boolean; // a viewer link: the HTTP transport is not its to open, so a proxy that drops WebSockets is said instead
};

// refusal is why the server would not open a stream, in its own words — another client holds the terminal — else the status
async function refusal(res: Response): Promise<string> {
  try {
    const body = (await res.json()) as { error?: string };
    if (body.error) return body.error;
  } catch {
    // no words: the status says it
  }
  return `open failed (${res.status})`;
}

// How long one batch of typing may take to go out, its upload counted: past it the person has moved on.
const INPUT_WAIT_MS = 5000;
const INPUT_CHARS_PER_MS = 50;

// What the terminal says by itself — mouse and focus reports, answers to the app's questions — is nobody's typing: when
// it cannot go, nothing need be said.
const REPORTS = /\x1b\[<[\d;]+[Mm]|\x1b\[M[\s\S]{3}|\x1b\[[IO]|\x1b\[[?>\d;]*[cnRt]|\x1b\[\?[\d;]*\$y|\x1bP[\s\S]*?\x1b\\|\x1b\][\s\S]*?(?:\x07|\x1b\\)/g;

// gridOf reads the socket's grid message, {"type":"grid","cols":…,"rows":…}; null for anything else.
export function gridOf(text: string): { cols: number; rows: number } | null {
  try {
    const msg = JSON.parse(text) as { type?: string; cols?: unknown; rows?: unknown };
    return msg.type === "grid" && typeof msg.cols === "number" && typeof msg.rows === "number" ? { cols: msg.cols, rows: msg.rows } : null;
  } catch {
    return null;
  }
}

// gridHeader reads the HTTP transport's X-Remuda-Grid, "<cols>x<rows>"; null when absent or malformed.
export function gridHeader(value: string | null): { cols: number; rows: number } | null {
  const m = /^(\d+)x(\d+)$/.exec(value ?? "");
  return m ? { cols: Number(m[1]), rows: Number(m[2]) } : null;
}

// typedIn is how much of the text the person typed.
const typedIn = (text: string) => text.replace(REPORTS, "").length;

// Once WebSockets prove unusable, every terminal on this page uses HTTP without trying again.
let preferHTTP = false;
let wsFailures = 0;
let wsRetry = 0;

// connect opens the stream and keeps it open; the transport behind the handle may change.
export function connect(stream: Stream): Transport {
  let closed = false;
  let current: Transport | null = null;
  const sizeText = () => {
    const { cols, rows } = stream.size();
    return `${cols}x${rows}`;
  };

  // wsWouldAccept tells a proxy that drops WebSockets from a server that ends the socket itself: a plain
  // GET of the same URL passes every check and fails only at the upgrade (426), so 426 means the proxy.
  const wsWouldAccept = () => fetch(`/ws/pane?${stream.query}`).then((r) => r.status === 426, () => false);

  const connectWS = (): Transport => {
    const proto = location.protocol === "https:" ? "wss" : "ws";
    const { cols, rows } = stream.size();
    const sentSize = `${cols}x${rows}`;
    let ws: WebSocket | null = new WebSocket(`${proto}://${location.host}/ws/pane?${stream.query}&cols=${cols}&rows=${rows}`);
    ws.binaryType = "arraybuffer";
    let opened = false;
    let retryTimer: ReturnType<typeof setTimeout> | null = null;
    const send = (obj: object) => {
      if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(obj));
    };
    const retry = () => {
      retryTimer = setTimeout(() => {
        if (!closed) current = connectWS();
      }, Math.min(5000, 250 * 2 ** wsRetry++));
    };
    ws.onopen = () => {
      opened = true;
      wsFailures = 0;
      stream.onOpen(sentSize);
    };
    ws.onmessage = (ev) => {
      if (typeof ev.data === "string") {
        const grid = gridOf(ev.data);
        if (grid) stream.onGrid?.(grid.cols, grid.rows);
        return;
      }
      if (!(ev.data instanceof ArrayBuffer)) return;
      wsRetry = 0;
      stream.onBytes(new Uint8Array(ev.data));
    };
    ws.onerror = () => ws?.close();
    ws.onclose = (ev) => {
      if (closed || ws === null) return;
      if (opened) {
        // a close reason (an attach refused: another client holds the terminal) is written on every
        // retry, since each one starts from a blank screen
        if (ev.code === 1000) stream.note("terminal detached — reconnecting");
        else if (ev.reason) stream.note(ev.reason);
        retry();
        return;
      }
      void wsWouldAccept().then((would) => {
        if (closed) return;
        if (would && stream.wsOnly) {
          stream.note("this network drops WebSockets, which a viewing link's live terminal needs");
        } else if (would && ++wsFailures >= 2) {
          preferHTTP = true;
          stream.note("WebSocket unavailable here — using the HTTP transport");
          current = connectHTTP();
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

  // connectHTTP opens a server-side stream, long-polls it for output and POSTs input through a sender of its own, one
  // request at a time so it stays in order; 410 means it expired. Typing goes now or not at all: what cannot go is dropped
  // and said, never kept for later, when the screen may be something else and a command would run unseen.
  const connectHTTP = (): Transport => {
    let sid: string | null = null;
    let type: ((data: string) => void) | null = null; // the open stream's typing; none between streams
    let stopped = false;
    let lossSaid = false;
    let reopenTimer: ReturnType<typeof setTimeout> | null = null;

    const lost = () => {
      if (lossSaid || stopped) return;
      lossSaid = true;
      stream.inputLost();
    };
    // typingTo is the typing of one stream: what it holds never reaches another
    const typingTo = (id: string) => {
      // positions in all that was handed to the sender: its end, where the last typing ended, and how far went through
      let pushed = 0;
      let typedTo = 0;
      let through = 0;
      const sender = createSender(
        async ({ text }) => {
          if (sid !== id) throw new Error("the stream is gone"); // what waited for it is not sent once it ended
          const abort = new AbortController();
          const timeout = setTimeout(() => abort.abort(), INPUT_WAIT_MS + text.length / INPUT_CHARS_PER_MS);
          try {
            const res = await fetch(`/api/term/input?sid=${id}`, { method: "POST", body: text, signal: abort.signal });
            if (!res.ok) throw new Error(`input refused (${res.status})`);
            through += text.length;
          } finally {
            clearTimeout(timeout);
          }
        },
        {
          sent: () => {
            lossSaid = false;
          },
          failed: () => {
            if (typedTo > through) lost();
            through = pushed; // the sender dropped the rest
          },
        },
      );
      return (data: string) => {
        pushed += data.length;
        if (typedIn(data) > 0) typedTo = pushed;
        sender.push({ text: data });
      };
    };
    const streamGone = () => {
      sid = null;
      type = null;
    };

    const closeStream = () => {
      if (!sid) return;
      void fetch(`/api/term/close?sid=${sid}`, { method: "DELETE", keepalive: true });
      streamGone();
    };
    const open = async () => {
      const sentSize = sizeText();
      const { cols, rows } = stream.size();
      const res = await fetch(`/api/term/open?${stream.query}&cols=${cols}&rows=${rows}`, { method: "POST" });
      if (!res.ok) throw new Error(await refusal(res));
      sid = ((await res.json()) as { sid: string }).sid;
      if (stopped) {
        closeStream(); // closed while opening: nothing may pin the attach
        return;
      }
      type = typingTo(sid);
      lossSaid = false;
      stream.onOpen(sentSize);
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
          stream.note(`cannot open the terminal stream (${e instanceof Error ? e.message : String(e)}) — retrying`);
          later(2000);
        }
        return;
      }
      while (!stopped && sid) {
        try {
          const res = await fetch(`/api/term/read?sid=${sid}&wait=20`);
          if (stopped) return;
          if (res.status === 410) {
            streamGone();
            later(500);
            return;
          }
          if (!res.ok) throw new Error(`read failed (${res.status})`);
          const buf = new Uint8Array(await res.arrayBuffer());
          if (stopped) return;
          const grid = gridHeader(res.headers.get("X-Remuda-Grid")); // the grid these bytes are drawn at, on every read
          if (grid) stream.onGrid?.(grid.cols, grid.rows);
          if (buf.length) stream.onBytes(buf);
          if (res.headers.get("X-Remuda-End") === "1") {
            // the server dropped it already; the attach ended (herdr restarted, say) and a fresh one is asked for, as the socket does
            streamGone();
            stream.note("terminal detached — reconnecting");
            later(500);
            return;
          }
        } catch {
          // a read that failed on the way may have taken bytes, the only repaint among them, that never reached the
          // screen: a fresh stream starts it over with a fresh paint, rather than reading on past the gap
          if (stopped) return;
          closeStream();
          later(1000);
          return;
        }
      }
    };
    void loop();
    return {
      input: (data) => {
        if (type) type(data);
        else if (typedIn(data) > 0) lost();
      },
      resize: (cols, rows) => {
        if (sid) void fetch(`/api/term/resize?sid=${sid}&cols=${cols}&rows=${rows}`, { method: "POST" });
      },
      close: () => {
        stopped = true;
        if (reopenTimer) clearTimeout(reopenTimer);
        closeStream();
      },
    };
  };

  current = preferHTTP && !stream.wsOnly ? connectHTTP() : connectWS();
  return {
    input: (data) => current?.input(data),
    resize: (cols, rows) => current?.resize(cols, rows),
    close: () => {
      closed = true;
      current?.close();
      current = null;
    },
  };
}
