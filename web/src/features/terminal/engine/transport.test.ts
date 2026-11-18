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
    const transport = connect({ query: "machine=local&id=w1:p1", size: () => ({ cols: 80, rows: 24 }), onOpen: (size) => opened.push(size), onBytes: () => {}, note: (text) => notes.push(text) });
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

// A stream on a fake clock: its opens answer as told, reads never do, and each POST of input takes the next answer.
function typing(answers: (() => Promise<Response>)[], opens: (() => Promise<Response>)[] = []) {
  const posted: string[] = [];
  const notes: string[] = [];
  const opened = () => Promise.resolve(new Response(JSON.stringify({ sid: "s1" }), { status: 200 }));
  vi.stubGlobal("WebSocket", DeadSocket);
  vi.stubGlobal("location", { protocol: "http:", host: "remuda" });
  vi.stubGlobal(
    "fetch",
    vi.fn((url: string, init?: RequestInit) => {
      const path = url.split("?")[0]!;
      if (path === "/ws/pane") return Promise.resolve(new Response("", { status: 426 }));
      if (path === "/api/term/open") return (opens.shift() ?? opened)();
      if (path !== "/api/term/input") return new Promise<Response>(() => {});
      posted.push(String(init?.body));
      const answer = answers.shift() ?? (() => Promise.resolve(new Response(null, { status: 204 })));
      return new Promise<Response>((resolve, reject) => {
        init?.signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")));
        void answer().then(resolve, reject);
      });
    }),
  );
  const transport: Transport = connect({ query: "machine=local&id=w1:p1", size: () => ({ cols: 80, rows: 24 }), onOpen: () => {}, onBytes: () => {}, note: (text) => notes.push(text) });
  return { transport, posted, notes, wait: (ms: number) => vi.advanceTimersByTimeAsync(ms) };
}

const LOST = "what you typed did not go through, and will not be sent again";

describe("typing over the HTTP transport", () => {
  beforeEach(() => vi.useFakeTimers());
  afterEach(() => {
    vi.useRealTimers();
    vi.unstubAllGlobals();
  });

  it("never sends again what failed, and says so once", async () => {
    for (const failure of [() => Promise.reject(new TypeError("offline")), () => Promise.resolve(new Response("", { status: 502 }))]) {
      const { transport, posted, notes, wait } = typing([failure, failure]);
      await wait(3000);
      transport.input("rm -rf build\r");
      await wait(60_000);
      expect(posted).toEqual(["rm -rf build\r"]);
      transport.input("x");
      await wait(100);
      expect(posted).toEqual(["rm -rf build\r", "x"]);
      expect(notes.filter((note) => note === LOST)).toHaveLength(1);
      transport.close();
    }
  });

  it("drops what is typed with no stream, and sends it neither then nor later", async () => {
    let open: (r: Response) => void = () => {};
    const { transport, posted, notes, wait } = typing([], [() => new Promise<Response>((resolve) => (open = resolve))]);
    await wait(3000);
    transport.input("rm -rf build\r");
    await wait(100);
    open(new Response(JSON.stringify({ sid: "s1" }), { status: 200 }));
    await wait(1000);
    expect(posted).toEqual([]);
    expect(notes).toContain(LOST);
    transport.close();
  });

  it("gives a new stream nothing typed before it opened", async () => {
    let open: (r: Response) => void = () => {};
    const { transport, posted, notes, wait } = typing([], [() => new Promise<Response>((resolve) => (open = resolve))]);
    await wait(3000);
    transport.input("rm -rf build\\r");
    open(new Response(JSON.stringify({ sid: "s1" }), { status: 200 }));
    await wait(1000);
    expect(posted).toEqual([]);
    expect(notes).toContain(LOST);
    transport.close();
  });

  it("gives up on a request that hangs, and what was typed behind it goes with it", async () => {
    const { transport, posted, notes, wait } = typing([() => new Promise<Response>(() => {})]);
    await wait(3000);
    transport.input("first");
    await wait(100);
    transport.input("second\r");
    await wait(60_000);
    expect(posted).toEqual(["first"]);
    expect(notes).toContain(LOST);
    transport.close();
  });

  it("keeps one request out at a time, in order", async () => {
    let answer: (r: Response) => void = () => {};
    const { transport, posted, wait } = typing([() => new Promise<Response>((resolve) => (answer = resolve))]);
    await wait(3000);
    transport.input("a");
    await wait(100);
    transport.input("b");
    transport.input("c");
    await wait(100);
    expect(posted).toEqual(["a"]);
    answer(new Response(null, { status: 204 }));
    await wait(100);
    expect(posted).toEqual(["a", "bc"]);
    transport.close();
  });
});
