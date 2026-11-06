// Web Push on the client: one subscription per browser, kept in step with the server's VAPID key.
// Subscribing is single-flight: two concurrent subscribe() calls make Chrome keep only the later
// one, so the first confirmation would hit a dead endpoint.

import { check, sendJSON } from "@/lib/http";
import { t } from "@/lib/i18n";

// Quiet is a daily window in this device's zone during which the server sends it nothing.
export type Quiet = { from: string; to: string; tz: string };

export type Prefs = { input: boolean; done: "off" | "long" | "always"; watchdog?: boolean; quiet?: Quiet };

export type PushState = {
  supported: boolean;
  reason?: string;
  permission: NotificationPermission | "unsupported";
  subscribed: boolean;
  endpoint?: string;
};

// an iPad on iPadOS reports a Mac's user agent; the touch points give it away
const isIOS = /iP(hone|ad|od)/.test(navigator.userAgent) || (navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1);
const standalone =
  matchMedia("(display-mode: standalone)").matches ||
  (navigator as unknown as { standalone?: boolean }).standalone === true;

function support(): { ok: boolean; reason?: string } {
  if (!("serviceWorker" in navigator)) return { ok: false, reason: "Needs HTTPS (or localhost): open remuda through tailscale serve." };
  if (!("PushManager" in window) || !("Notification" in window)) {
    if (isIOS && !standalone) return { ok: false, reason: "On iPhone and iPad, add remuda to the Home Screen first — push only works in the installed app." };
    return { ok: false, reason: "This browser has no Web Push." };
  }
  return { ok: true };
}

async function registration(): Promise<ServiceWorkerRegistration> {
  // `ready` never settles when registration failed; don't hang forever
  return await Promise.race([
    navigator.serviceWorker.ready,
    new Promise<never>((_, reject) => setTimeout(() => reject(new Error("service worker not ready")), 10000)),
  ]);
}

async function currentSub(): Promise<PushSubscription | null> {
  return (await registration()).pushManager.getSubscription();
}

async function serverKey(): Promise<string> {
  const res = await check(await fetch("/api/push"));
  return ((await res.json()) as { public_key: string }).public_key;
}

async function register(sub: PushSubscription, prefs: Prefs, oldEndpoint?: string): Promise<void> {
  await sendJSON("POST", "/api/push/subscribe", {
    subscription: sub.toJSON(),
    prefs,
    label: navigator.userAgent.slice(0, 120),
    ...(oldEndpoint ? { old_endpoint: oldEndpoint } : {}),
  });
}

export async function getPushState(): Promise<PushState> {
  const sup = support();
  if (!sup.ok) return { supported: false, reason: sup.reason, permission: "unsupported", subscribed: false };
  try {
    const sub = await currentSub();
    return { supported: true, permission: Notification.permission, subscribed: !!sub, endpoint: sub?.endpoint };
  } catch (e) {
    return { supported: false, reason: e instanceof Error ? e.message : String(e), permission: Notification.permission, subscribed: false };
  }
}

let inflight: Promise<void> | null = null;

// enablePush asks for permission, subscribes (anew when the server's key changed) and registers the subscription.
export function enablePush(prefs: Prefs): Promise<void> {
  if (inflight) return inflight;
  inflight = (async () => {
    try {
      const sup = support();
      if (!sup.ok) throw new Error(sup.reason);
      const perm = await Notification.requestPermission();
      if (perm !== "granted") throw new Error("Notifications are blocked for this site in the browser settings.");
      const reg = await registration();
      const key = urlBase64ToUint8Array(await serverKey());
      let sub = await reg.pushManager.getSubscription();
      if (sub && !sameKey(sub, key)) {
        await sub.unsubscribe();
        sub = null;
      }
      if (!sub) {
        sub = await subscribeOrSay(reg, key);
      }
      await register(sub, prefs);
    } finally {
      inflight = null;
    }
  })();
  return inflight;
}

// SUBSCRIBE_TIMEOUT_MS is how long the browser's push service may take to register: a browser without one may never answer
const SUBSCRIBE_TIMEOUT_MS = 20_000;

// subscribeOrSay registers with the browser's push service, or says in the person's words why it could not
async function subscribeOrSay(reg: ServiceWorkerRegistration, key: Uint8Array<ArrayBuffer>): Promise<PushSubscription> {
  try {
    return await Promise.race([
      reg.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: key }),
      new Promise<never>((_, reject) => setTimeout(() => reject(new Error(t("The browser's push service gave no answer in 20 s."))), SUBSCRIBE_TIMEOUT_MS)),
    ]);
  } catch (e) {
    const reason = e instanceof Error ? e.message : String(e);
    throw new Error(t("The browser would not register for push ({reason}). A private window, a headless browser or one without a push service cannot get pushes; the alerts still show on this page while it is in front.", { reason }));
  }
}

export async function updatePrefs(prefs: Prefs): Promise<void> {
  const sub = await currentSub();
  if (!sub) throw new Error("not subscribed");
  await register(sub, prefs);
}

// disablePush tells the server first (needs the cookie), then the browser.
export async function disablePush(): Promise<void> {
  const sub = await currentSub();
  if (!sub) return;
  await sendJSON("DELETE", "/api/push/subscribe", { endpoint: sub.endpoint });
  await sub.unsubscribe();
}

export function sendTestPush(endpoint: string): Promise<void> {
  return sendJSON("POST", "/api/push/test", { endpoint });
}

// resyncPush re-registers silently on every load with permission granted, so a server that lost its state regains this
// device; a subscription made under another key than the server's — a new state folder, a cockpit moved to another
// machine under the same address — can never be delivered to, so it is replaced with one under the server's key.
export function resyncPush(prefs: Prefs): Promise<void> {
  if (!support().ok || Notification.permission !== "granted") return Promise.resolve();
  if (inflight) return inflight;
  inflight = (async () => {
    try {
      const reg = await registration();
      const sub = await reg.pushManager.getSubscription();
      if (!sub) return;
      const key = urlBase64ToUint8Array(await serverKey());
      if (sameKey(sub, key)) return await register(sub, prefs);
      const old = sub.endpoint;
      await sub.unsubscribe();
      await register(await subscribeOrSay(reg, key), prefs, old);
    } catch {
      /* best effort */
    } finally {
      inflight = null;
    }
  })();
  return inflight;
}

function sameKey(sub: PushSubscription, key: Uint8Array<ArrayBuffer>): boolean {
  const cur = sub.options.applicationServerKey;
  if (!cur) return false;
  const a = new Uint8Array(cur);
  if (a.length !== key.length) return false;
  for (let i = 0; i < a.length; i++) if (a[i] !== key[i]) return false;
  return true;
}

// urlBase64ToUint8Array decodes the server's key over a plain ArrayBuffer: newer TS libs type
// `new Uint8Array(n)` as Uint8Array<ArrayBufferLike>, which BufferSource no longer accepts.
function urlBase64ToUint8Array(base64: string): Uint8Array<ArrayBuffer> {
  const padding = "=".repeat((4 - (base64.length % 4)) % 4);
  const b64 = (base64 + padding).replace(/-/g, "+").replace(/_/g, "/");
  const raw = atob(b64);
  const out = new Uint8Array(new ArrayBuffer(raw.length));
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
  return out;
}
