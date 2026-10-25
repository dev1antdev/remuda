// useWelcomeBack stamps when the page was last seen — on hiding, on leaving, every minute while visible — and, on a return
// after an hour or more, reads the standup since that moment and hands the shell one toast line; nothing when nothing
// happened, one toast per return.

import { useEffect } from "react";
import { actions } from "@/app/store";
import { readPref, writePref } from "@/lib/prefs";
import { fetchStandup } from "./api";
import { awayGap, awayLine, LAST_SEEN_KEY } from "./away";

const STAMP_EVERY = 60_000;

export function useWelcomeBack(): void {
  useEffect(() => {
    const stamp = () => writePref(LAST_SEEN_KEY, new Date().toISOString());
    let greetedFor = "";
    const check = () => {
      if (document.visibilityState !== "visible") return;
      const lastSeen = readPref<string>(LAST_SEEN_KEY, "");
      const gap = awayGap(lastSeen, Date.now());
      stamp();
      if (gap === null || greetedFor === lastSeen) return;
      greetedFor = lastSeen;
      const tz = Intl.DateTimeFormat().resolvedOptions().timeZone || "UTC";
      fetchStandup(lastSeen, tz)
        .then((standup) => {
          const line = awayLine(standup.items, gap);
          if (line) actions.setAway(line);
        })
        .catch(() => {});
    };
    check();
    const every = setInterval(() => {
      if (document.visibilityState === "visible") stamp();
    }, STAMP_EVERY);
    const onVisibility = () => (document.visibilityState === "hidden" ? stamp() : check());
    document.addEventListener("visibilitychange", onVisibility);
    window.addEventListener("pagehide", stamp);
    return () => {
      clearInterval(every);
      document.removeEventListener("visibilitychange", onVisibility);
      window.removeEventListener("pagehide", stamp);
    };
  }, []);
}
