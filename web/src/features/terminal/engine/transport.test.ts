import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { connect, type Transport } from "./transport";

// DeadSocket is a WebSocket a proxy drops: it closes before it ever opens
class DeadSocket {
  static OPEN = 1;
  binaryType = "";
  readyState = 0;
  onopen: (() => void) | null = null;
  onmessage: ((ev: unknown) => void) | null = null;
  onerror: (() => void) | null = null;
  onclose: ((ev: { code: number; reason: string }) => void) | null = null;
  constructor() {
    setTimeout(() => this.onclose?.({ code: 1006, reason: "" }), 0);
  }
  send() {}
  close() {}
}

type Answer = { status: number; body?: string; headers?: Record<string, string> };

// server answers the transport's requests in turn by route: the proxy probe, opens and reads
function server(opens: Answer[], reads: Answer[]) {
  const asked: string[] = [];
  const fetch = vi.fn(async (url: string) => {
    const path = url.split("?")[0]!;
    asked.push(path);
    const next = path === "/ws/pane" ? { status: 426 } : path === "/api/term/open" ? (opens.shift() ?? { status: 503 }) : path === "/api/term/read" ? (reads.shift() ?? null) : { status: 204 };
    if (next === null) return new Promise<Response>(() => {}); // a long poll that never answers
    return new Response(next.status === 204 ? null : (next.body ?? ""), { status: next.status, headers: next.headers });
  });
  return { fetch, asked };
}

describe("the HTTP transport", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("says why an open was refused and asks for a fresh attach once the old one ended", async () => {
    const { fetch, asked } = server(
      [{ status: 409, body: JSON.stringify({ error: "this terminal already has an attached client" }) }, { status: 200, body: JSON.stringify({ sid: "s1" }) }, { status: 200, body: JSON.stringify({ sid: "s2" }) }],
      [{ status: 200, body: "bye", headers: { "X-Remuda-End": "1" } }],
    );
    vi.stubGlobal("WebSocket", DeadSocket);
    vi.stubGlobal("location", { protocol: "http:", host: "remuda" });
    vi.stubGlobal("fetch", fetch);
    const notes: string[] = [];
    const opened: string[] = [];
    const transport = connect({ query: "machine=local&id=w1:p1", size: () => ({ cols: 80, rows: 24 }), onOpen: (size) => opened.push(size), onBytes: () => {}, note: (text) => notes.push(text), inputLost: () => {} });
    try {
      await vi.waitFor(() => expect(opened.length).toBe(2), { timeout: 6000, interval: 20 });
    } finally {
      transport.close();
    }
    expect(notes).toContain("cannot open the terminal stream (this terminal already has an attached client) — retrying");
    expect(notes).toContain("terminal detached — reconnecting");
    expect(asked.filter((path) => path === "/api/term/open")).toHaveLength(3);
  }, 10000);
});

describe("the grid over HTTP", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("is applied before each read's bytes, and a read lost on the way loses no grid", async () => {
    const { fetch } = server(
      [{ status: 200, body: JSON.stringify({ sid: "s1" }) }, { status: 200, body: JSON.stringify({ sid: "s2" }) }],
      [
        { status: 200, body: "first", headers: { "X-Remuda-Grid": "100x30" } },
        { status: 502 }, // the read that carried the change never arrives
        { status: 200, body: "after", headers: { "X-Remuda-Grid": "90x28" } },
      ],
    );
    vi.stubGlobal("WebSocket", DeadSocket);
    vi.stubGlobal("location", { protocol: "http:", host: "remuda" });
    vi.stubGlobal("fetch", fetch);
    const seen: string[] = [];
    const decoder = new TextDecoder();
    const transport = connect({
      query: "machine=local&id=w1:p1",
      size: () => ({ cols: 40, rows: 10 }),
      onOpen: () => {},
      onBytes: (buf) => seen.push(`bytes ${decoder.decode(buf)}`),
      onGrid: (cols, rows) => seen.push(`grid ${cols}x${rows}`),
      note: () => {},
      inputLost: () => {},
      freshOnLoss: true,
    });
    try {
      await vi.waitFor(() => expect(seen).toContain("bytes after"), { timeout: 8000, interval: 20 });
    } finally {
      transport.close();
    }
    expect(seen).toEqual(["grid 100x30", "bytes first", "grid 90x28", "bytes after"]);
  }, 10000);
});

describe("a read lost on the way", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("on herdr reads on from the same stream, the screen and its scrollback kept", async () => {
    const { fetch, asked } = server(
      [{ status: 200, body: JSON.stringify({ sid: "s1" }) }, { status: 200, body: JSON.stringify({ sid: "s2" }) }],
      [{ status: 502 }, { status: 200, body: "more" }],
    );
    vi.stubGlobal("WebSocket", DeadSocket);
    vi.stubGlobal("location", { protocol: "http:", host: "remuda" });
    vi.stubGlobal("fetch", fetch);
    let opened = 0;
    const painted: string[] = [];
    const decoder = new TextDecoder();
    const transport = connect({ query: "machine=local&id=w1:p1", size: () => ({ cols: 80, rows: 24 }), onOpen: () => opened++, onBytes: (buf) => painted.push(decoder.decode(buf)), note: () => {}, inputLost: () => {} });
    try {
      await vi.waitFor(() => expect(painted).toContain("more"), { timeout: 8000, interval: 20 });
    } finally {
      transport.close();
    }
    expect(opened).toBe(1);
    expect(asked.filter((path) => path === "/api/term/open")).toHaveLength(1);
  }, 10000);

  it("on remuda-core drops its stream for a fresh one, so an idle terminal is painted again", async () => {
    const { fetch, asked } = server(
      [{ status: 200, body: JSON.stringify({ sid: "s1" }) }, { status: 200, body: JSON.stringify({ sid: "s2" }) }],
      [
        { status: 502 }, // took the only repaint with it
        { status: 200, body: "fresh paint" }, // the new stream's first paint; the app prints nothing more
      ],
    );
    vi.stubGlobal("WebSocket", DeadSocket);
    vi.stubGlobal("location", { protocol: "http:", host: "remuda" });
    vi.stubGlobal("fetch", fetch);
    let opened = 0;
    const painted: string[] = [];
    const decoder = new TextDecoder();
    const transport = connect({ query: "machine=local&id=w1:p1", size: () => ({ cols: 80, rows: 24 }), onOpen: () => opened++, onBytes: (buf) => painted.push(decoder.decode(buf)), note: () => {}, inputLost: () => {}, freshOnLoss: true });
    try {
      await vi.waitFor(() => expect(painted).toContain("fresh paint"), { timeout: 8000, interval: 20 });
    } finally {
      transport.close();
    }
    expect(opened).toBe(2);
    expect(asked.filter((path) => path === "/api/term/close").length).toBeGreaterThanOrEqual(1);
  }, 10000);
});

describe("a viewer link's transport", () => {
  afterEach(() => vi.unstubAllGlobals());

  it("never opens the HTTP stream a viewer may not, and says a proxy drops its WebSockets", async () => {
    const { fetch, asked } = server([], []);
    vi.stubGlobal("WebSocket", DeadSocket);
    vi.stubGlobal("location", { protocol: "http:", host: "remuda" });
    vi.stubGlobal("fetch", fetch);
    const notes: string[] = [];
    const transport = connect({ query: "machine=local&id=w1:p1", size: () => ({ cols: 80, rows: 24 }), onOpen: () => {}, onBytes: () => {}, note: (text) => notes.push(text), inputLost: () => {}, wsOnly: true });
    try {
      await vi.waitFor(() => expect(asked.filter((path) => path === "/ws/pane").length).toBeGreaterThanOrEqual(2), { timeout: 6000, interval: 20 });
    } finally {
      transport.close();
    }
    expect(notes).toContain("this network drops WebSockets, which a viewing link's live terminal needs");
    expect(asked).not.toContain("/api/term/open");
  }, 10000);
});

// A stream on a fake clock: opens and reads answer as told (a read not told never answers), each POST of input takes the
// next answer, and every stream opened has its own id.
function typing(answers: (() => Promise<Response>)[], { opens = [], reads = [] }: { opens?: (() => Promise<Response>)[]; reads?: (() => Promise<Response>)[] } = {}) {
  const posted: string[] = [];
  const to: string[] = [];
  let lost = 0;
  let streams = 0;
  const opened = () => Promise.resolve(new Response(JSON.stringify({ sid: `s${++streams}` }), { status: 200 }));
  vi.stubGlobal("WebSocket", DeadSocket);
  vi.stubGlobal("location", { protocol: "http:", host: "remuda" });
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      const [path, query] = url.split("?") as [string, string];
      if (path === "/ws/pane") return Promise.resolve(new Response("", { status: 426 }));
      if (path === "/api/term/open") return (opens.shift() ?? opened)();
      if (path === "/api/term/read") return reads.shift()?.() ?? new Promise<Response>(() => {});
      if (path !== "/api/term/input") return Promise.resolve(new Response(null, { status: 204 }));
      posted.push(String(init?.body));
      to.push(new URLSearchParams(query).get("sid") ?? "");
      const answer = answers.shift() ?? (() => Promise.resolve(new Response(null, { status: 204 })));
      return new Promise<Response>((resolve, reject) => {
        init?.signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")));
        void answer().then(resolve, reject);
      });
    }),
  );
  const transport: Transport = connect({ query: "machine=local&id=w1:p1", size: () => ({ cols: 80, rows: 24 }), onOpen: () => {}, onBytes: () => {}, note: () => {}, inputLost: () => lost++ });
  return { transport, posted, to, lost: () => lost, wait: (ms: number) => vi.advanceTimersByTimeAsync(ms) };
}

const refused = () => Promise.resolve(new Response("", { status: 502 }));
const offline = () => Promise.reject(new TypeError("offline"));
const never = () => new Promise<Response>(() => {});

describe("typing over the HTTP transport", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("never sends again what failed, and says so once", async () => {
    for (const failure of [offline, refused]) {
      const { transport, posted, lost, wait } = typing([failure, failure]);
      await wait(3000);
      transport.input("rm -rf build\r");
      await wait(60_000);
      expect(posted).toEqual(["rm -rf build\r"]);
      transport.input("x");
      await wait(100);
      expect(posted).toEqual(["rm -rf build\r", "x"]);
      expect(lost()).toBe(1);
      transport.close();
    }
  });

  it("drops what is typed while the stream opens, the answer still on its way included", async () => {
    let head: (r: Response) => void = () => {};
    let body: (v: unknown) => void = () => {};
    const { transport, posted, lost, wait } = typing([], { opens: [() => new Promise<Response>((resolve) => (head = resolve))] });
    await wait(3000);
    transport.input("rm -rf");
    expect(lost()).toBe(1);
    head({ ok: true, status: 200, json: () => new Promise((resolve) => (body = resolve)) } as unknown as Response);
    await wait(10);
    transport.input(" build\r");
    body({ sid: "s1" });
    await wait(1000);
    expect(posted).toEqual([]);
    transport.input("ls\r");
    await wait(10);
    expect(posted).toEqual(["ls\r"]);
    transport.close();
  });

  it("gives up on a request that hangs, and what was typed behind it goes with it", async () => {
    const { transport, posted, lost, wait } = typing([never]);
    await wait(3000);
    transport.input("first");
    await wait(100);
    transport.input("second\r");
    await wait(60_000);
    expect(posted).toEqual(["first"]);
    expect(lost()).toBe(1);
    transport.close();
  });

  it("cuts a big paste into requests the server takes, in order", async () => {
    const { transport, posted, lost, wait } = typing([]);
    await wait(3000);
    transport.input("x".repeat(500_000));
    await wait(100);
    if (lost() !== 0) throw new Error("a paste within the limits was lost");
    if (posted.length < 2 || posted.join("").length !== 500_000) {
      throw new Error(`posted ${posted.length} requests of ${posted.map((p) => p.length).join("+")}`);
    }
    if (posted.some((p) => p.length > 64 * 1024)) throw new Error("a request past the server's 1 MiB limit");
    transport.close();
  });

  it("keeps a stream's typing to that stream, and an old failure off the new one", async () => {
    let fail: (e: unknown) => void = () => {};
    let end: (r: Response) => void = () => {};
    const { transport, posted, to, lost, wait } = typing([() => new Promise<Response>((_, reject) => (fail = reject))], { reads: [() => new Promise<Response>((resolve) => (end = resolve))] });
    await wait(3000);
    transport.input("old");
    await wait(10);
    transport.input(" behind");
    end(new Response("", { status: 410 }));
    await wait(10);
    transport.input("between");
    expect(lost()).toBe(1);
    await wait(1000);
    fail(new TypeError("offline"));
    await wait(10);
    expect(lost()).toBe(2); // said again on the new stream: this was another loss
    transport.input("new");
    await wait(100);
    expect(posted).toEqual(["old", "new"]);
    expect(to).toEqual(["s1", "s2"]);
    transport.close();
  });

  it("sends nothing that waited once its stream ended, and says so", async () => {
    let answer: (r: Response) => void = () => {};
    let end: (r: Response) => void = () => {};
    const { transport, posted, to, lost, wait } = typing([() => new Promise<Response>((resolve) => (answer = resolve))], { reads: [() => new Promise<Response>((resolve) => (end = resolve))] });
    await wait(3000);
    transport.input("a");
    await wait(10);
    transport.input("b\r");
    end(new Response("", { status: 410 }));
    await wait(1000);
    answer(new Response(null, { status: 204 }));
    await wait(100);
    expect(posted).toEqual(["a"]);
    expect(to).toEqual(["s1"]);
    expect(lost()).toBe(1);
    transport.close();
  });

  it("sends nothing that waited once the terminal closed", async () => {
    let answer: (r: Response) => void = () => {};
    const { transport, posted, lost, wait } = typing([() => new Promise<Response>((resolve) => (answer = resolve))]);
    await wait(3000);
    transport.input("a");
    await wait(10);
    transport.input("rm -rf build\r");
    transport.close();
    answer(new Response(null, { status: 204 }));
    await wait(100);
    expect(posted).toEqual(["a"]);
    expect(lost()).toBe(0);
  });

  it("says a loss when typing fails behind reports that went", async () => {
    let answer: (r: Response) => void = () => {};
    const wheel = "\x1b[<65;10;5M".repeat(400);
    const { transport, posted, lost, wait } = typing([() => new Promise<Response>((resolve) => (answer = resolve)), () => Promise.resolve(new Response(null, { status: 204 })), refused]);
    await wait(3000);
    transport.input("\x1b[I");
    await wait(10);
    transport.input(wheel);
    transport.input("rm\r");
    answer(new Response(null, { status: 204 }));
    await wait(100);
    expect(posted.length).toBe(3);
    expect(lost()).toBe(1);
    transport.close();
  });

  it("says no loss when only a report failed after the typing went", async () => {
    let answer: (r: Response) => void = () => {};
    const { transport, posted, lost, wait } = typing([() => new Promise<Response>((resolve) => (answer = resolve)), () => Promise.resolve(new Response(null, { status: 204 })), refused]);
    await wait(3000);
    transport.input("a");
    await wait(10);
    transport.input("\x1b]"); // Alt+], and Ctrl+G: together they read like a report
    transport.input("\x07");
    answer(new Response(null, { status: 204 }));
    await wait(100);
    transport.input("\x1b[I");
    await wait(100);
    expect(posted).toEqual(["a", "\x1b]\x07", "\x1b[I"]);
    expect(lost()).toBe(0);
    transport.close();
  });

  it("counts what a failure dropped as settled", async () => {
    const { transport, lost, wait } = typing([refused, () => Promise.resolve(new Response(null, { status: 204 })), refused]);
    await wait(3000);
    transport.input("x");
    await wait(10);
    transport.input("y");
    await wait(10);
    transport.input("\x1b[I");
    await wait(10);
    expect(lost()).toBe(1);
    transport.close();
  });

  it("says nothing of the terminal's own reports", async () => {
    const wheel = "\x1b[<65;10;5M";
    const { transport, posted, lost, wait } = typing([offline], { opens: [() => new Promise<Response>(() => {})] });
    await wait(3000);
    transport.input(wheel);
    transport.input("\x1b[I");
    transport.input("\x1b[12;40R");
    expect(lost()).toBe(0);
    transport.close();
    const open = typing([offline]);
    await open.wait(3000);
    open.transport.input(wheel);
    await open.wait(100);
    expect(open.posted).toEqual([wheel]);
    expect(open.lost()).toBe(0);
    open.transport.close();
    expect(posted).toEqual([]);
  });
});
