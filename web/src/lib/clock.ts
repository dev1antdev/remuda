// A clock as a hook: "now" refreshed every so many milliseconds, for a counter that runs while the piece is on screen.

import { useEffect, useState } from "react";

// useNow is the time, refreshed every `everyMs` while the component is mounted
export function useNow(everyMs: number): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), everyMs);
    return () => clearInterval(id);
  }, [everyMs]);
  return now;
}
