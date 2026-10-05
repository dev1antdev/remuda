// herdr-ui service worker: network-first for navigations (a fresh build always
// wins online), cache-first for content-addressed assets, and Web Push.
// /api and /ws are never intercepted. Nothing is precached.
const CACHE = "herdr-ui-v2";

self.addEventListener("install", () => self.skipWaiting());

self.addEventListener("activate", (event) => {
  event.waitUntil(
    (async () => {
      const names = await caches.keys();
      await Promise.all(names.filter((n) => n !== CACHE).map((n) => caches.delete(n)));
      await self.clients.claim();
    })(),
  );
});

self.addEventListener("fetch", (event) => {
  const url = new URL(event.request.url);
  if (url.origin !== location.origin) return;
  if (url.pathname.startsWith("/api/") || url.pathname.startsWith("/ws/")) return;

  if (event.request.mode === "navigate") {
    event.respondWith(
      (async () => {
        try {
          const res = await fetch(event.request);
          // only a real shell goes in — never the 401 sign-in page
          if (res.ok) {
            const cache = await caches.open(CACHE);
            cache.put("/", res.clone());
          }
          return res;
        } catch {
          const cached = await caches.match("/");
          return cached ?? Response.error();
        }
      })(),
    );
    return;
  }

  if (url.pathname.startsWith("/assets/") || url.pathname.startsWith("/icons/")) {
    event.respondWith(
      (async () => {
        const cached = await caches.match(event.request);
        if (cached) return cached;
        const res = await fetch(event.request);
        if (res.ok) {
          const cache = await caches.open(CACHE);
          cache.put(event.request, res.clone());
        }
        return res;
      })(),
    );
  }
});

// ---- push ----

// Every push shows a notification: Safari revokes subscriptions whose pushes
// stay silent. With a window visible it is shown quietly (the app already
// shows the status) and replaces the pane's earlier one.
self.addEventListener("push", (event) => {
  let data = {};
  try {
    data = event.data ? event.data.json() : {};
  } catch {
    data = { title: "herdr-ui", body: event.data ? event.data.text() : "" };
  }
  event.waitUntil(
    (async () => {
      const wins = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
      const visible = wins.some((w) => w.visibilityState === "visible");
      await self.registration.showNotification(data.title || "herdr-ui", {
        body: data.body || "",
        tag: data.tag || "herdr-ui",
        renotify: true,
        silent: visible,
        icon: "/icons/icon-192.png",
        badge: "/icons/icon-192.png",
        data: { machine: data.machine, pane: data.pane },
      });
    })(),
  );
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const { machine, pane } = event.notification.data || {};
  event.waitUntil(
    (async () => {
      const wins = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
      const win = wins.find((w) => w.visibilityState === "visible") || wins[0];
      if (win) {
        try {
          await win.focus();
        } catch {
          /* iOS may refuse; the message still lands */
        }
        win.postMessage({ type: "select-pane", machine, pane });
        return;
      }
      const url =
        machine && pane
          ? `/?machine=${encodeURIComponent(machine)}&pane=${encodeURIComponent(pane)}`
          : "/";
      await self.clients.openWindow(url);
    })(),
  );
});

// The browser rotated the subscription: re-subscribe with the same server
// key and tell the server which endpoint this replaces, so the device keeps
// its alert preferences. Without this, pushes die until the app is reopened.
self.addEventListener("pushsubscriptionchange", (event) => {
  event.waitUntil(
    (async () => {
      const old = event.oldSubscription;
      let key = old && old.options && old.options.applicationServerKey;
      if (!key) {
        const res = await fetch("/api/push");
        if (!res.ok) return;
        key = (await res.json()).public_key; // the Push API accepts the base64url string as is
      }
      const sub = await self.registration.pushManager.subscribe({ userVisibleOnly: true, applicationServerKey: key });
      await fetch("/api/push/subscribe", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ subscription: sub.toJSON(), old_endpoint: old ? old.endpoint : "" }),
      });
    })(),
  );
});
