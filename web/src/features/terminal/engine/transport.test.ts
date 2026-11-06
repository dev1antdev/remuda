import { afterEach, describe, expect, it, vi } from "vitest";
import { connect } from "./transport";

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
