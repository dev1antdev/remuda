// One socket per page for the server's word that something moved — the roster, a pane's transcript — so the screens
// reload at once and poll only as a safety net (lib/query.ts). The page tells the socket which panes it shows; the
// socket closes while nothing of ours is visible and comes back with a growing pause. The hooks never see a socket,
// only events and the connected flag.

import { useEffect, useRef } from "react";
import { createStore } from "./store";
import { pageVisible, watchVisibility } from "./visibility";

export type LiveTopic = "roster" | "chat";
export type LiveEvent = { topic: LiveTopic; machine?: string; pane?: string; version?: string };

const state = createStore<{ connected: boolean }>({ connected: false });

// useLiveConnected says whether the socket is up: a screen polls slower then
export const useLiveConnected = (): boolean => state.use((s) => s.connected);
export const liveConnected = (): boolean => state.get().connected;

// backoffDelay is the pause before the attempt-th reconnect: a second, doubling, half a minute at most
export function backoffDelay(attempt: number): number {
  return Math.min(30000, 1000 * 2 ** Math.max(0, Math.min(attempt, 10)));
}

// paneKey names a pane the way the hub does
export const paneKey = (machine: string, pane: string): string => `${machine}|${pane}`;

// subscriptionFrame is what the page tells the hub it shows: every watched key once, sorted
export function subscriptionFrame(watched: Map<string, number>): string {
  const sub = [...watched.entries()]
    .filter(([, count]) => count > 0)
    .map(([key]) => key)
    .sort();
  return JSON.stringify({ sub });
}

// parseEvent reads one frame; null for anything that is not an event
export function parseEvent(text: string): LiveEvent | null {
  try {
    const value: unknown = JSON.parse(text);
    if (!value || typeof value !== "object") return null;
    const { topic, machine, pane, version } = value as Record<string, unknown>;
    if (topic !== "roster" && topic !== "chat") return null;
    return {
      topic,
      machine: typeof machine === "string" ? machine : undefined,
      pane: typeof pane === "string" ? pane : undefined,
      version: typeof version === "string" ? version : undefined,
    };
  } catch {
    return null;
  }
}

// slowerPoll is the pace a screen keeps while the socket carries the news: ten seconds for a quick poll, thirty for a slow one
export function slowerPoll(every: number, connected: boolean): number {
  if (!connected) return every;
  return every <= 3000 ? 10000 : 30000;
}

const listeners = new Map<LiveTopic, Set<(event: LiveEvent) => void>>();

// onEvent hears every event of a topic; the return stops it
export function onEvent(topic: LiveTopic, fn: (event: LiveEvent) => void): () => void {
  let set = listeners.get(topic);
  if (!set) listeners.set(topic, (set = new Set()));
  set.add(fn);
  return () => {
    set.delete(fn);
  };
}

function dispatch(event: LiveEvent) {
  for (const fn of listeners.get(event.topic) ?? []) fn(event);
}

// useLive hears a topic for the life of the component; the handler may change without resubscribing
export function useLive(topic: LiveTopic, fn: (event: LiveEvent) => void): void {
  const ref = useRef(fn);
  ref.current = fn;
  useEffect(() => onEvent(topic, (event) => ref.current(event)), [topic]);
}

// ---- the panes the page shows ----

const watched = new Map<string, number>();
let frameTimer: ReturnType<typeof setTimeout> | null = null;

// watchPane counts one more viewer of the pane; the return lets go. The frame goes out once per tick, so a wall of tiles mounting costs one.
export function watchPane(machine: string, pane: string): () => void {
  const key = paneKey(machine, pane);
  watched.set(key, (watched.get(key) ?? 0) + 1);
  scheduleFrame();
  return () => {
    const count = (watched.get(key) ?? 1) - 1;
    if (count <= 0) watched.delete(key);
    else watched.set(key, count);
    scheduleFrame();
  };
}

function scheduleFrame() {
  if (frameTimer) return;
  frameTimer = setTimeout(() => {
    frameTimer = null;
    sendFrame();
  }, 50);
}

function sendFrame() {
  if (socket && socket.readyState === WebSocket.OPEN) socket.send(subscriptionFrame(watched));
}

// ---- the socket ----

let socket: WebSocket | null = null;
let attempt = 0;
let retry: ReturnType<typeof setTimeout> | null = null;
let started = false;

// startLive opens the stream once for the page; every later call is a no-op
export function startLive(): void {
  if (started || typeof WebSocket === "undefined") return;
  started = true;
  watchVisibility(onVisibility);
  connect();
}

function connect() {
  if (socket || !pageVisible()) return;
  const proto = location.protocol === "https:" ? "wss" : "ws";
  const ws = new WebSocket(`${proto}://${location.host}/ws/events`);
  socket = ws;
  ws.onopen = () => {
    attempt = 0;
    state.set({ connected: true });
    sendFrame();
  };
  ws.onmessage = (message) => {
    const event = parseEvent(String(message.data));
    if (event) dispatch(event);
  };
  ws.onclose = () => {
    if (socket === ws) socket = null;
    state.set({ connected: false });
    scheduleReconnect();
  };
}

function scheduleReconnect() {
  if (retry || !pageVisible()) return;
  retry = setTimeout(() => {
    retry = null;
    connect();
  }, backoffDelay(attempt++));
}

// onVisibility closes the socket while nothing of ours is on screen and reopens it at once on return
function onVisibility() {
  if (pageVisible()) {
    if (retry) {
      clearTimeout(retry);
      retry = null;
    }
    if (!socket) {
      attempt = 0;
      connect();
    }
  } else {
    if (retry) {
      clearTimeout(retry);
      retry = null;
    }
    socket?.close();
  }
}
