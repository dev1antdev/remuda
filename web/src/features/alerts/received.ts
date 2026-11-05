// What this browser knows of its pushes: the last one the service worker received (kept in IndexedDB by public/sw.js,
// told to the open windows as it lands), what the cockpit last handed its push service, and a short chime.

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

// chime is two soft notes through the browser's own audio, for an alert shown on the page; silent where audio is refused
export function chime(): void {
  try {
    const audio = new AudioContext();
    const notes = [660, 880];
    notes.forEach((frequency, i) => {
      const tone = audio.createOscillator();
      const volume = audio.createGain();
      const start = audio.currentTime + i * 0.12;
      tone.frequency.value = frequency;
      volume.gain.setValueAtTime(0.0001, start);
      volume.gain.exponentialRampToValueAtTime(0.08, start + 0.02);
      volume.gain.exponentialRampToValueAtTime(0.0001, start + 0.18);
      tone.connect(volume).connect(audio.destination);
      tone.start(start);
      tone.stop(start + 0.2);
    });
    setTimeout(() => void audio.close(), 600);
  } catch {
    /* no audio here */
  }
}
