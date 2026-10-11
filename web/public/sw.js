// remuda service worker: network-first for navigations (a fresh build always
// wins online), cache-first for the content-addressed bundle under /assets/
// and the app icons, and Web Push. /api, /ws and the editor under /code/
// are never intercepted. Nothing is precached.
const CACHE = "remuda-v2";

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
  if (url.pathname.startsWith("/api/") || url.pathname.startsWith("/ws/") || url.pathname === "/code" || url.pathname.startsWith("/code/")) return;

  // a cache write outlives the response; the worker stays alive for it
  const keep = (req, res) => event.waitUntil(caches.open(CACHE).then((cache) => cache.put(req, res)));

  if (event.request.mode === "navigate") {
    event.respondWith(
      (async () => {
        try {
          const res = await fetch(event.request);
          // only the app shell at "/" goes in, and only a real one — never
          // the 401 sign-in page, never some other path's answer
          if (res.ok && url.pathname === "/") keep("/", res.clone());
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
        if (res.ok) keep(event.request, res.clone());
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
    data = { title: "remuda", body: event.data ? event.data.text() : "" };
  }
  event.waitUntil(
    (async () => {
      const wins = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
      const visible = wins.some((w) => w.visibilityState === "visible");
      await self.registration.showNotification(data.title || "remuda", {
        body: data.body || "",
        tag: data.tag || "remuda",
        renotify: true,
        silent: visible,
        icon: "/icons/icon-192.png",
        badge: "/icons/icon-192.png",
        actions: Array.isArray(data.actions) ? data.actions.slice(0, 2) : [],
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
      // a button on a watchdog notification: Esc goes straight to the pane
      if (event.action === "esc" && machine && pane) {
        await fetch("/api/keys", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ machine, pane, keys: ["esc"] }),
        }).catch(() => {});
        return;
      }
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
