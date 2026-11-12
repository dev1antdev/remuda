// remuda service worker: network-first for navigations, cache-first for the content-addressed bundle
// under /assets/ and the app icons, and Web Push. /api, /ws and the editor under /code/ are never
// intercepted; nothing is precached.
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
          // only the app shell at "/" goes in, and only a real one: never the 401 sign-in page, never another path's answer
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

// isPage says whether a window shows the cockpit's page, the one that puts an alert up in its own corner and answers a
// tap: a tab of the same origin on the editor under /code/ or on a file under /api/ shows neither
function isPage(win) {
  let path;
  try {
    path = new URL(win.url).pathname;
  } catch {
    return false;
  }
  return win.frameType !== "nested" && !path.startsWith("/api/") && !path.startsWith("/ws/") && path !== "/code" && !path.startsWith("/code/");
}

// pages are the open windows that show the cockpit's page
async function pages() {
  return (await self.clients.matchAll({ type: "window", includeUncontrolled: true })).filter(isPage);
}

// Every push shows a notification: Safari, Firefox and Chrome take a push that shows none for a silent one, and
// revoke the subscription or show their own. A window of ours that has the focus shows the alert itself, as a toast
// in its corner, so there the notification is taken down again at once; a visible one that lacks the focus (behind
// another window, on a second screen) gets it quietly. A test push always stays: it is what the person asked to see.
// The last push is noted for the alerts panel, and told to the open windows.
self.addEventListener("push", (event) => {
  let data = {};
  try {
    data = event.data ? event.data.json() : {};
  } catch {
    data = { title: "remuda", body: event.data ? event.data.text() : "" };
  }
  event.waitUntil(
    (async () => {
      const wins = await pages();
      const visible = wins.some((w) => w.visibilityState === "visible");
      const focused = wins.some((w) => w.focused);
      const tag = data.tag || "remuda";
      await self.registration.showNotification(data.title || "remuda", {
        body: data.body || "",
        tag,
        renotify: true,
        silent: visible,
        icon: "/icons/icon-192.png",
        badge: "/icons/icon-192.png",
        actions: Array.isArray(data.actions) ? data.actions.slice(0, 2) : [],
        data: { machine: data.machine, pane: data.pane, panel: data.panel, choices: Array.isArray(data.choices) ? data.choices : [], dialog: data.dialog || "", offer: data.offer || "" },
      });
      const onPage = focused && data.kind !== "test";
      if (onPage) for (const shown of await self.registration.getNotifications({ tag })) shown.close();
      const note = { at: Date.now(), title: data.title || "remuda", kind: data.kind || "", shown: onPage ? "page" : "os" };
      await notePush(note);
      for (const win of wins) win.postMessage({ type: "pushed", ...note });
    })(),
  );
});

// notePush keeps the last push in IndexedDB, where the alerts panel reads it: "a push came at 14:02" tells a push
// that never arrived from one the system did not show
function notePush(note) {
  if (typeof indexedDB === "undefined") return Promise.resolve();
  return new Promise((resolve) => {
    const open = indexedDB.open("remuda", 1);
    open.onupgradeneeded = () => open.result.createObjectStore("kv");
    open.onerror = () => resolve();
    open.onsuccess = () => {
      const db = open.result;
      const tx = db.transaction("kv", "readwrite");
      tx.objectStore("kv").put(note, "lastPush");
      tx.oncomplete = tx.onerror = () => {
        db.close();
        resolve();
      };
    };
  });
}

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const { machine, pane, panel, choices, dialog, offer } = event.notification.data || {};
  event.waitUntil(
    (async () => {
      // a plan limit's offer: the held card or schedule runs on the other vendor's agent; refused (taken, its window
      // reset, the card moved) or unreachable, the page opens on the panel the push names, where the offers stand
      if (event.action === "offer-run" && offer) {
        const res = await fetch(`/api/limits/offers/${encodeURIComponent(offer)}/run`, { method: "POST", headers: { "Content-Type": "application/json" }, body: "{}" }).catch(() => null);
        if (res && res.ok) return;
      }
      // a button on a watchdog notification: Esc goes straight to the pane; refused or unreachable, the page opens
      // on the pane instead, as for a choice, so the person sees it did not land
      if (event.action === "esc" && machine && pane) {
        const res = await fetch("/api/keys", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({ machine, pane, keys: ["esc"] }),
        }).catch(() => null);
        if (res && res.ok) return;
      }
      // a choice on a question notification: the server checks the screen still shows that very dialog (its key) with
      // that choice, then picks it; a dialog that changed or closed is a refusal, and the page opens on the pane instead
      if (/^choice-\d$/.test(event.action) && machine && pane) {
        const index = Number(event.action.slice("choice-".length));
        const label = Array.isArray(choices) ? choices[index] : undefined;
        if (label) {
          const res = await fetch("/api/question/answer", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ machine, pane, index, label, dialog }),
          }).catch(() => null);
          if (res && res.ok) return;
        }
      }
      const wins = await pages();
      const win = wins.find((w) => w.visibilityState === "visible") || wins[0];
      if (win) {
        try {
          await win.focus();
        } catch {
          /* iOS may refuse; the message still lands */
        }
        // a digest names a panel of the page (the standup) instead of a pane
        win.postMessage(panel ? { type: "open-panel", panel } : { type: "select-pane", machine, pane });
        return;
      }
      const url = panel
        ? `/?panel=${encodeURIComponent(panel)}`
        : machine && pane
          ? `/?machine=${encodeURIComponent(machine)}&pane=${encodeURIComponent(pane)}`
          : "/";
      await self.clients.openWindow(url);
    })(),
  );
});

// The browser rotated the subscription: re-subscribe with the same server key and tell the server which
// endpoint this replaces, so the device keeps its prefs; without this, pushes die until the app is reopened.
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
