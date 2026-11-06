// Office mode's choice of the agent shown large — the pinned one, else whoever needs the person, else a rotation every
// OFFICE_ROTATE from one timer set at the last switch, nothing polled — and the full screen it lives in: the page's own
// is kept while the mode is on, and when Esc or the browser's exit ends it, the mode ends with it.

import { useCallback, useEffect, useRef, useState } from "react";
import type { RadarPrefs } from "@/features/roster/radar";
import { OFFICE_ROTATE, featured, neighbour, rotates, type Tile } from "@/features/roster/wall";
import { actions, app } from "./store";

export type Office = {
  key: string; // the tile shown large, "" when the wall is empty
  pinned: string; // the tile pinned by hand, "" when the mode picks
  pin: (key: string) => void; // pin this tile, or unpin it when it is the pinned one
  step: (by: 1 | -1) => void; // pin the next or the previous tile in the wall's order
};

export function useOffice(on: boolean, shown: Tile[], radar: RadarPrefs, setOn: (on: boolean) => void): Office {
  const [pinned, setPinned] = useState("");
  const [show, setShow] = useState({ key: "", since: 0 });
  const [, wake] = useState(0);
  const now = Date.now();
  const key = on ? featured(shown, pinned, show.key, show.since, now, radar) : "";
  useEffect(() => {
    if (key !== show.key) setShow({ key, since: Date.now() });
  }, [key, show.key]);
  const rotating = on && rotates(shown, pinned, now, radar);
  useEffect(() => {
    if (!rotating) return;
    const id = setTimeout(() => wake((n) => n + 1), Math.max(show.since + OFFICE_ROTATE - Date.now(), 250));
    return () => clearTimeout(id);
  }, [rotating, show.since]);

  const full = app.use((s) => s.full);
  const wasFull = useRef(false);
  useEffect(() => {
    if (!on) {
      wasFull.current = false;
      setPinned("");
      return;
    }
    if (full) {
      wasFull.current = true;
      return;
    }
    if (wasFull.current) {
      wasFull.current = false;
      setOn(false); // Esc or the browser's own exit ended the full screen: office mode goes with it
    } else {
      actions.setFull(true); // the page came back to the Wall in office mode: the page's full screen (the browser's needs a gesture)
    }
  }, [on, full, setOn]);

  const latest = useRef({ key, shown });
  latest.current = { key, shown };
  const pin = useCallback((k: string) => setPinned((p) => (p === k ? "" : k)), []);
  const step = useCallback((by: 1 | -1) => setPinned(neighbour(latest.current.shown, latest.current.key, by)), []);
  return { key, pinned, pin, step };
}
