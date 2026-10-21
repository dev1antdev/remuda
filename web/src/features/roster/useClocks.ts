// The two clocks the radar rows and the wall tiles share: the spinner's frame and "now" for the idle tiers.

import { useEffect, useState } from "react";
import type { Machine } from "./api";
import { SPIN_MS } from "./radar";

// useSpinner is the one clock behind every braille frame and pulsing question mark, ticking only while something moves
export function useSpinner(moving: boolean): number {
  const [step, setStep] = useState(0);
  useEffect(() => {
    if (!moving) return;
    const id = setInterval(() => setStep((s) => (s + 1) % 1_000_000), SPIN_MS);
    return () => clearInterval(id);
  }, [moving]);
  return step;
}

// useClock is "now" for the idle tiers: every half minute, and at every roster change
export function useClock(machines: Machine[]): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 30_000);
    return () => clearInterval(id);
  }, []);
  useEffect(() => setNow(Date.now()), [machines]);
  return now;
}

// moving says whether any agent on the machines works or asks, the spinner's reason to tick
export const moving = (machines: Machine[]) => machines.some((m) => m.sessions.some((s) => s.tabs.some((t) => t.panes.some((p) => p.radar?.state === "working" || p.radar?.state === "blocked"))));
