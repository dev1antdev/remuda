import { describe, expect, it } from "vitest";
import source from "../../public/sw.js?raw";

type Answer = { ok: boolean; status: number } | Error;
type Window = { url: string; visibilityState: string; focused?: boolean; frameType?: string; focus: () => Promise<void>; postMessage: (message: unknown) => void };
type Shown = { title: string; options: { tag: string; silent: boolean }; closed: boolean };

// worker runs sw.js against stand-ins for the worker's globals: what it fetched, opened, posted and showed is kept
function worker(answer: Answer, windows: Window[] = []) {
  const handlers: Record<string, (event: unknown) => void> = {};
  const fetched: { url: string; body: unknown }[] = [];
  const opened: string[] = [];
  const shown: Shown[] = [];
  const self = {
    addEventListener: (type: string, handler: (event: unknown) => void) => (handlers[type] = handler),
    skipWaiting: () => {},
    clients: { matchAll: async () => windows, openWindow: async (url: string) => void opened.push(url), claim: async () => {} },
    registration: {
      showNotification: async (title: string, options: Shown["options"]) => void shown.push({ title, options, closed: false }),
      getNotifications: async ({ tag }: { tag: string }) => shown.filter((s) => s.options.tag === tag && !s.closed).map((s) => ({ close: () => (s.closed = true) })),
    },
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
  const push = async (data: Record<string, unknown>) => {
    let done: Promise<unknown> = Promise.resolve();
    handlers.push!({ data: { json: () => data }, waitUntil: (promise: Promise<unknown>) => (done = promise) });
    await done;
  };
  return { click, push, fetched, opened, shown };
}

const page = (visibilityState: string, focused: boolean, posted: unknown[] = [], url = "http://127.0.0.1:7317/?machine=local&pane=w1%3Ap1"): Window => ({ url, visibilityState, focused, focus: async () => {}, postMessage: (message) => posted.push(message) });

// editor is a tab of the same origin that is not the cockpit's page: the editor's own tab, opened from the Code mode
const editor = (visibilityState: string, focused: boolean, posted: unknown[] = []): Window => page(visibilityState, focused, posted, "http://127.0.0.1:7317/code/local/?folder=/tmp/x");

describe("a push", () => {
  const alert = { title: "api needs you", body: "claude is waiting", tag: "local|w1:p1", kind: "input", machine: "local", pane: "w1:p1" };
  it("shows a notification with a sound when no window of ours is open, and keeps it", async () => {
    const sw = worker({ ok: true, status: 200 });
    await sw.push(alert);
    expect(sw.shown).toMatchObject([{ title: "api needs you", options: { tag: "local|w1:p1", silent: false }, closed: false }]);
  });
  it("shows it quietly beside a visible window that lacks the focus, and keeps it", async () => {
    const sw = worker({ ok: true, status: 200 }, [page("visible", false)]);
    await sw.push(alert);
    expect(sw.shown).toMatchObject([{ options: { silent: true }, closed: false }]);
  });
  it("takes it down again when a window of ours has the focus, which shows the alert itself, and tells that window", async () => {
    const posted: unknown[] = [];
    const sw = worker({ ok: true, status: 200 }, [page("visible", true, posted)]);
    await sw.push(alert);
    expect(sw.shown).toMatchObject([{ closed: true }]);
    expect(posted).toMatchObject([{ type: "pushed", title: "api needs you", kind: "input", shown: "page" }]);
  });
  it("keeps it, with its sound, when the window that has the focus is the editor's tab or a file, which show no alert", async () => {
    const posted: unknown[] = [];
    const sw = worker({ ok: true, status: 200 }, [editor("visible", true, posted), page("hidden", false), page("visible", true, [], "http://127.0.0.1:7317/api/fs/file?machine=local&path=/tmp/a.png")]);
    await sw.push(alert);
    expect(sw.shown).toMatchObject([{ options: { silent: false }, closed: false }]);
    expect(posted).toEqual([]);
  });
  it("keeps a question's choices and its dialog's key on the notification, for a tap to send back", async () => {
    const sw = worker({ ok: true, status: 200 });
    await sw.push({ ...alert, actions: [{ action: "choice-0", title: "Yes" }], choices: ["Yes", "No"], dialog: "0f1e2d3c4b5a6978" });
    expect(sw.shown).toMatchObject([{ options: { data: { machine: "local", pane: "w1:p1", choices: ["Yes", "No"], dialog: "0f1e2d3c4b5a6978" } } }]);
  });
  it("keeps a test push even over a focused window: it is what the person asked to see", async () => {
    const posted: unknown[] = [];
    const sw = worker({ ok: true, status: 200 }, [page("visible", true, posted)]);
    await sw.push({ title: "remuda", body: "Alerts reach this device.", tag: "test", kind: "test" });
    expect(sw.shown).toMatchObject([{ closed: false }]);
    expect(posted).toMatchObject([{ type: "pushed", kind: "test", shown: "os" }]);
  });
});

const pane = { machine: "local", pane: "w1:p1", choices: ["Yes", "No"], dialog: "0f1e2d3c4b5a6978" };

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
    const sw = worker({ ok: false, status: 409 }, [page("hidden", false, posted)]);
    await sw.click("esc", pane);
    expect(posted).toEqual([{ type: "select-pane", machine: "local", pane: "w1:p1" }]);
    expect(sw.opened).toEqual([]);
  });
  it("brings up the page rather than the editor's tab in front of it, and opens one when only the editor is open", async () => {
    const onPage: unknown[] = [];
    const onEditor: unknown[] = [];
    const both = worker({ ok: false, status: 409 }, [editor("visible", true, onEditor), page("hidden", false, onPage)]);
    await both.click("esc", pane);
    expect(onPage).toEqual([{ type: "select-pane", machine: "local", pane: "w1:p1" }]);
    expect(onEditor).toEqual([]);
    const alone = worker({ ok: false, status: 409 }, [editor("visible", true, onEditor)]);
    await alone.click("esc", pane);
    expect(alone.opened).toEqual(["/?machine=local&pane=w1%3Ap1"]);
    expect(onEditor).toEqual([]);
  });
});

describe("a notification's choice", () => {
  it("answers through the server, and opens the pane when the dialog moved", async () => {
    const taken = worker({ ok: true, status: 200 });
    await taken.click("choice-1", pane);
    expect(taken.fetched[0]).toEqual({ url: "/api/question/answer", body: { machine: "local", pane: "w1:p1", index: 1, label: "No", dialog: "0f1e2d3c4b5a6978" } });
    expect(taken.opened).toEqual([]);
    const moved = worker({ ok: false, status: 409 });
    await moved.click("choice-0", pane);
    expect(moved.opened).toEqual(["/?machine=local&pane=w1%3Ap1"]);
  });
});
