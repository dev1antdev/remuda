// useHistory is the composer's memory of what was sent from this device to a folder: ↑ walks back through it, ↓
// forward and back to the line that was being written, a send remembers.

import { useCallback, useRef } from "react";
import { usePref } from "@/lib/prefs";
import { recall, remember } from "./history";

export function useHistory(machineId: string, dir: string) {
  const [list, setList] = usePref<string[]>(`prompt.history:${machineId}|${dir}`, []);
  const at = useRef(-1); // where the walk is: -1 is the live draft
  const stashed = useRef(""); // the draft as it was when the walk began
  const walk = useCallback(
    (direction: "up" | "down", draft: string): string | null => {
      if (at.current < 0) stashed.current = draft;
      const next = recall(list, at.current, direction, stashed.current);
      if (!next) return null;
      at.current = next.at;
      return next.text;
    },
    [list],
  );
  const reset = useCallback(() => {
    at.current = -1;
  }, []);
  const add = useCallback(
    (text: string) => {
      at.current = -1;
      setList((old) => remember(old, text));
    },
    [setList],
  );
  return { walk, reset, add, browsing: () => at.current >= 0 };
}
