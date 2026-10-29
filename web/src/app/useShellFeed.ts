// useShellFeed keeps the Mac app's menu bar in step with the roster: one "ready" when the first roster is in, and a
// "menu" message whenever the agents that need the person, or the counts of the others, change. Nothing outside the app.

import { useEffect, useRef } from "react";
import { DEFAULT_RADAR, type RadarPrefs } from "@/features/roster/radar";
import { useRoster } from "@/features/roster/useRoster";
import { usePref } from "@/lib/prefs";
import { inShell, tellShell } from "@/lib/shell";
import { sameFeed, shellRows, type ShellFeed } from "./shellFeed";

export function useShellFeed(): void {
  const native = inShell();
  const { machines, version } = useRoster();
  const [radar] = usePref<RadarPrefs>("radar", DEFAULT_RADAR);
  const ready = useRef(false);
  const last = useRef<ShellFeed | null>(null);

  useEffect(() => {
    if (!native || ready.current || !version) return;
    ready.current = true;
    tellShell({ type: "ready", version, machine: machines.find((m) => m.id === "local")?.label ?? "local" });
  }, [native, version, machines]);

  useEffect(() => {
    if (!native || !version) return; // nothing before the first roster: an empty menu would only flash
    const feed = shellRows(machines, radar, Date.now());
    if (last.current && sameFeed(last.current, feed)) return;
    last.current = feed;
    tellShell({ type: "menu", ...feed });
  }, [native, version, machines, radar]);
}
