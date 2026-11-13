// What this browser knows of its pushes: the last one the service worker received (kept in IndexedDB by public/sw.js,
// told to the open windows as it lands), and what the cockpit last handed its push service.

import { sendJSON } from "@/lib/http";
import type { Delivery, Received } from "./device";
import type { Prefs } from "./push";

// readLastPush is the push the service worker noted last, null when none came or the store cannot be read
export function readLastPush(): Promise<Received | null> {
  if (typeof indexedDB === "undefined") return Promise.resolve(null);
  return new Promise((resolve) => {
    const open = indexedDB.open("remuda", 1);
    open.onupgradeneeded = () => open.result.createObjectStore("kv");
    open.onerror = () => resolve(null);
    open.onsuccess = () => {
      const db = open.result;
      try {
        const get = db.transaction("kv", "readonly").objectStore("kv").get("lastPush");
        get.onsuccess = () => resolve((get.result as Received | undefined) ?? null);
        get.onerror = () => resolve(null);
      } catch {
        resolve(null);
      } finally {
        db.close();
      }
    };
  });
}

// onPushed calls back for every push the service worker receives while this page is open; the return stops listening
export function onPushed(callback: (received: Received) => void): () => void {
  if (!("serviceWorker" in navigator)) return () => {};
  const listen = (event: MessageEvent) => {
    const data = event.data as ({ type?: string } & Received) | null;
    if (data?.type === "pushed") callback({ at: data.at, title: data.title, kind: data.kind, shown: data.shown });
  };
  navigator.serviceWorker.addEventListener("message", listen);
  return () => navigator.serviceWorker.removeEventListener("message", listen);
}

// DeviceOnServer is what the cockpit holds for this browser's subscription
export type DeviceOnServer = { known: boolean; prefs?: Prefs; last?: Delivery | null };

export const fetchDevice = (endpoint: string): Promise<DeviceOnServer> => sendJSON("POST", "/api/push/device", { endpoint });
