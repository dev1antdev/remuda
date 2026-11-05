// The alerts shown on the page: the roster carries the activity's newest id, and the live socket moves it the moment
// an alert is recorded; past the one this page saw last, while the page is in front, the new entries are read and the
// ones this device's switches want go up as toasts. The first roster only seeds, so a reload says nothing of the past;
// alerts that came while the page was away are left to the push and to the welcome back. The Mac app has its own
// notifications and a viewer link's screen is for looking, so neither shows them here.

import { useEffect, useRef } from "react";
import { actions } from "@/app/store";
import { fetchFeed } from "@/features/activity/api";
import { useRoster } from "@/features/roster/useRoster";
import { usePref } from "@/lib/prefs";
import { inShell } from "@/lib/shell";
import { alertOf, newAlerts } from "./onpage";
import { readAlertPrefs } from "./prefs";
import { chime } from "./received";

// SOUND_KEY is the per-device switch for a chime with an alert shown on the page, off unless turned on
export const SOUND_KEY = "alerts.sound";

export function useOnPageAlerts(): void {
  const { feed, viewOnly } = useRoster();
  const [sound] = usePref<boolean>(SOUND_KEY, false);
  const seen = useRef<number | null>(null);
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
        const shown = newAlerts(entries, after, readAlertPrefs());
        for (const entry of shown) actions.addAlert(alertOf(entry));
        if (shown.length > 0 && sound) chime();
      })
      .catch(() => {});
  }, [feed, viewOnly, sound]);
}
