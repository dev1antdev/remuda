import { describe, expect, it } from "vitest";
import source from "../../public/sw.js?raw";

type Answer = { ok: boolean; status: number } | Error;
type Window = { visibilityState: string; focus: () => Promise<void>; postMessage: (message: unknown) => void };

// worker runs sw.js against stand-ins for the worker's globals: what it fetched, opened and posted is kept
function worker(answer: Answer, windows: Window[] = []) {
  const handlers: Record<string, (event: unknown) => void> = {};
  const fetched: { url: string; body: unknown }[] = [];
  const opened: string[] = [];
  const self = {
    addEventListener: (type: string, handler: (event: unknown) => void) => (handlers[type] = handler),
    skipWaiting: () => {},
    clients: { matchAll: async () => windows, openWindow: async (url: string) => void opened.push(url), claim: async () => {} },
    registration: { showNotification: async () => {} },
  };
  const fetchStandIn = async (url: string, init?: { body?: string }) => {
    fetched.push({ url, body: init?.body ? JSON.parse(init.body) : undefined });
    if (answer instanceof Error) throw answer;
    return answer;
  };
  new Function("self", "fetch", "caches", "location", source)(self, fetchStandIn, {}, { origin: "http://127.0.0.1:7317" });
  const click = async (action: string, data: Record<string, unknown>) => {
    let done: Promise<unknown> = Promise.resolve();
    handlers.notificationclick!({ action, notification: { close: () => {}, data }, waitUntil: (promise: Promise<unknown>) => (done = promise) });
    await done;
  };
  return { click, fetched, opened };
}

const pane = { machine: "local", pane: "w1:p1", choices: ["Yes", "No"] };

describe("the notification's Esc", () => {
  it("sends Esc to the pane and opens nothing when the server takes it", async () => {
    const sw = worker({ ok: true, status: 200 });
    await sw.click("esc", pane);
    expect(sw.fetched).toEqual([{ url: "/api/keys", body: { machine: "local", pane: "w1:p1", keys: ["esc"] } }]);
    expect(sw.opened).toEqual([]);
  });
  it("opens the page on the pane when the server refuses it or cannot be reached, as a choice does", async () => {
    for (const answer of [{ ok: false, status: 409 }, { ok: false, status: 401 }, new Error("offline")]) {
      const sw = worker(answer);
      await sw.click("esc", pane);
      expect(sw.opened, String(answer instanceof Error ? answer.message : answer.status)).toEqual(["/?machine=local&pane=w1%3Ap1"]);
    }
  });
  it("brings an open window to the pane instead of a new one", async () => {
    const posted: unknown[] = [];
    const sw = worker({ ok: false, status: 409 }, [{ visibilityState: "hidden", focus: async () => {}, postMessage: (message) => posted.push(message) }]);
    await sw.click("esc", pane);
    expect(posted).toEqual([{ type: "select-pane", machine: "local", pane: "w1:p1" }]);
    expect(sw.opened).toEqual([]);
  });
});

describe("a notification's choice", () => {
  it("answers through the server, and opens the pane when the dialog moved", async () => {
    const taken = worker({ ok: true, status: 200 });
    await taken.click("choice-1", pane);
    expect(taken.fetched[0]).toEqual({ url: "/api/question/answer", body: { machine: "local", pane: "w1:p1", index: 1, label: "No" } });
    expect(taken.opened).toEqual([]);
    const moved = worker({ ok: false, status: 409 });
    await moved.click("choice-0", pane);
    expect(moved.opened).toEqual(["/?machine=local&pane=w1%3Ap1"]);
  });
});
