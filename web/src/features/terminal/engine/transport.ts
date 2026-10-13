// The pane's byte stream: a WebSocket, or HTTP long-polling once WebSockets prove unusable on this page
// (a proxy that drops them). Bytes come in, keystrokes and resizes go back; a dropped connection retries.

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
  note: (text: string) => void; // a line on the screen from remuda itself
};

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
        if (would && ++wsFailures >= 2) {
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
      const sentSize = sizeText();
      const { cols, rows } = stream.size();
      const res = await fetch(`/api/term/open?${stream.query}&cols=${cols}&rows=${rows}`, { method: "POST" });
      if (!res.ok) throw new Error(`open failed (${res.status})`);
      sid = ((await res.json()) as { sid: string }).sid;
      if (stopped) {
        closeStream(); // closed while opening: nothing may pin the attach
        return;
      }
      stream.onOpen(sentSize);
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
            sid = null;
            later(500);
            return;
          }
          if (!res.ok) throw new Error(`read failed (${res.status})`);
          const buf = new Uint8Array(await res.arrayBuffer());
          if (stopped) return;
          if (buf.length) stream.onBytes(buf);
          if (res.headers.get("X-Remuda-End") === "1") {
            sid = null; // the server dropped it already
            stream.note("terminal detached");
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

  current = preferHTTP ? connectHTTP() : connectWS();
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
