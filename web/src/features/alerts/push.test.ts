import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

// browser runs push.ts against stand-ins for the browser's push globals: a registration holding one subscription made
// under `subscribedWith`, a server whose key is `serverKey`; what was posted to the server is kept
function browser(subscribedWith: number[], serverKey: number[]) {
  const posted: { url: string; body: unknown }[] = [];
  const made: string[] = [];
  let unsubscribed = 0;
  const subscription = (endpoint: string, key: number[]) => ({
    endpoint,
    options: { applicationServerKey: new Uint8Array(key).buffer },
    toJSON: () => ({ endpoint, keys: { p256dh: "p", auth: "a" } }),
    unsubscribe: async () => {
      unsubscribed++;
      current = null;
      return true;
    },
  });
  let current: ReturnType<typeof subscription> | null = subscription("https://push.example/old", subscribedWith);
  const pushManager = {
    getSubscription: async () => current,
    subscribe: async ({ applicationServerKey }: { applicationServerKey: Uint8Array }) => {
      current = subscription("https://push.example/new", [...applicationServerKey]);
      made.push(current.endpoint);
      return current;
    },
  };
  const base64url = btoa(String.fromCharCode(...serverKey)).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
  vi.stubGlobal("navigator", { userAgent: "Chrome/140", platform: "MacIntel", maxTouchPoints: 0, serviceWorker: { ready: Promise.resolve({ pushManager }) } });
  vi.stubGlobal("window", { PushManager: function PushManager() {}, Notification: {} });
  vi.stubGlobal("Notification", { permission: "granted" });
  vi.stubGlobal("matchMedia", () => ({ matches: false }));
  vi.stubGlobal("fetch", async (url: string, init?: { body?: string }) => {
    if (url === "/api/push") return new Response(JSON.stringify({ public_key: base64url }), { status: 200 });
    posted.push({ url, body: init?.body ? JSON.parse(init.body) : undefined });
    return new Response(JSON.stringify({ ok: true }), { status: 200 });
  });
  return { posted, made, unsubscribed: () => unsubscribed };
}

const prefs = { input: true, done: "long" as const, watchdog: true };

describe("resyncPush", () => {
  beforeEach(() => vi.resetModules());
  afterEach(() => vi.unstubAllGlobals());

  it("registers the subscription the browser holds when it was made with the server's key", async () => {
    const b = browser([1, 2, 3], [1, 2, 3]);
    const { resyncPush } = await import("./push");
    await resyncPush(prefs);
    expect(b.unsubscribed()).toBe(0);
    expect(b.posted).toMatchObject([{ url: "/api/push/subscribe", body: { subscription: { endpoint: "https://push.example/old" }, prefs } }]);
  });

  it("turned on again under a changed key, names the subscription it replaces, so the server stops pushing to the dead one", async () => {
    const b = browser([1, 2, 3], [9, 8, 7]);
    vi.stubGlobal("Notification", { permission: "granted", requestPermission: async () => "granted" });
    const { enablePush } = await import("./push");
    await enablePush(prefs);
    expect(b.made).toEqual(["https://push.example/new"]);
    expect(b.posted).toMatchObject([{ url: "/api/push/subscribe", body: { subscription: { endpoint: "https://push.example/new" }, old_endpoint: "https://push.example/old" } }]);
  });

  it("subscribes anew when the server's key changed, since the old subscription can never be delivered to", async () => {
    const b = browser([1, 2, 3], [9, 8, 7]);
    const { resyncPush } = await import("./push");
    await resyncPush(prefs);
    expect(b.unsubscribed()).toBe(1);
    expect(b.made).toEqual(["https://push.example/new"]);
    expect(b.posted).toMatchObject([{ url: "/api/push/subscribe", body: { subscription: { endpoint: "https://push.example/new" }, prefs, old_endpoint: "https://push.example/old" } }]);
  });
});
