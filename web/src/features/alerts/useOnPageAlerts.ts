// The alerts shown on the page: the roster carries the activity's newest id, and the live socket moves it the moment
// an alert is recorded; past the one this page saw last, while the page is in front, the new entries are read and the
// ones this device's switches want go up as toasts. The first roster only seeds, so a reload says nothing of the past;
// alerts that came while the page was away are left to the push and to the welcome back. The Mac app has its own
// notifications and a viewer link's screen is for looking, so neither shows them here. A question's toast goes once
// the roster says its pane no longer asks; the chime keeps quiet inside the device's quiet hours.

import { useEffect, useRef } from "react";
import { actions, app } from "@/app/store";
import { fetchFeed } from "@/features/activity/api";
import type { Machine } from "@/features/roster/api";
import { useRoster } from "@/features/roster/useRoster";
import { usePref } from "@/lib/prefs";
import { inShell } from "@/lib/shell";
import { alertOf, newAlerts, quietNow, stillStanding } from "./onpage";
import { readAlertPrefs } from "./prefs";
import { chime } from "./received";

// SOUND_KEY is the per-device switch for a chime with an alert shown on the page, off unless turned on
export const SOUND_KEY = "alerts.sound";

// asksIn says whether the roster shows a pane asking: blocked by the radar, or herdr's status "input"
const asksIn = (machines: Machine[]) => (machineId: string, paneId: string) =>
  machines
    .find((machine) => machine.id === machineId)
    ?.sessions.some((session) => session.tabs.some((tab) => tab.panes.some((pane) => pane.id === paneId && (pane.radar?.state === "blocked" || pane.status === "input")))) ?? false;

export function useOnPageAlerts(): void {
  const { feed, viewOnly, machines } = useRoster();
  const [sound] = usePref<boolean>(SOUND_KEY, false);
  const seen = useRef<number | null>(null);
  const roster = useRef(machines);
  roster.current = machines;
  useEffect(() => {
    if (feed === undefined) return;
    if (seen.current === null || feed < seen.current) {
      seen.current = feed; // the first roster, or a server whose activity started over
      return;
    }
    if (feed === seen.current) return;
    const after = seen.current;
    seen.current = feed;
    if (viewOnly || inShell() || document.visibilityState !== "visible") return;
    void fetchFeed(20)
      .then((entries) => {
        const prefs = readAlertPrefs();
        const shown = stillStanding(newAlerts(entries, after, prefs).map(alertOf), asksIn(roster.current));
        for (const alert of shown) actions.addAlert(alert);
        if (shown.length > 0 && sound && !quietNow(prefs.quiet)) chime();
      })
      .catch(() => {});
  }, [feed, viewOnly, sound]);
  useEffect(() => {
    const standing = app.get().alerts;
    if (standing.length === 0) return;
    const kept = stillStanding(standing, asksIn(machines));
    if (kept !== standing) actions.setAlerts(kept);
  }, [machines]);
}
