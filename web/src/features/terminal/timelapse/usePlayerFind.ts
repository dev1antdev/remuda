// usePlayerFind is the player's find over the recordings it lists: the field's text, the steps through every place
// the text stood, the one on screen, and a step either way, which picks the recording and takes the player to the
// frame. A new answer moves to its first step — on the recording on screen when it has one — unless the step on
// screen is among its steps (a hit the search panel opened the player at).

import { useEffect, useMemo, useRef, useState } from "react";
import { stepAt, stepFrom, stepsOf, unreadWords, type Step, type TimelapseFound } from "./found";
import { useScreenSearch } from "./useScreenSearch";

type Scope = { machineId: string; paneId: string; dir: string; shown: string[]; chosen: string; found?: TimelapseFound; pick: (recording: string) => void };

export function usePlayerFind({ machineId, paneId, dir, shown, chosen, found, pick }: Scope) {
  const [text, setText] = useState(found?.query ?? "");
  const [jump, setJump] = useState<Step | null>(found ? { recording: found.recording, frame: found.frame, at: found.at, line: found.line } : null);
  const search = useScreenSearch(text, { machine: machineId, pane: paneId, dir });
  const hits = search.data?.hits;
  const order = shown.join("|");
  const steps = useMemo(() => stepsOf(hits ?? [], shown), [hits, order]); // eslint-disable-line react-hooks/exhaustive-deps
  const at = stepAt(steps, jump);
  const go = (index: number) => {
    const next = steps[index];
    if (!next) return;
    setJump({ ...next }); // a step of its own each time, so the player goes back to the frame even when it is the same one
    if (next.recording !== chosen) pick(next.recording);
  };
  const answered = useRef<unknown>(null);
  useEffect(() => {
    if (!search.data || answered.current === search.data) return;
    answered.current = search.data;
    if (steps.length === 0 || stepAt(steps, jump) >= 0) return;
    const here = steps.findIndex((s) => s.recording === chosen);
    go(here >= 0 ? here : 0);
  }, [search.data, steps]); // eslint-disable-line react-hooks/exhaustive-deps
  return {
    text,
    setText,
    jump,
    steps,
    at,
    step: (by: 1 | -1) => go(stepFrom(steps.length, at, by)),
    forget: () => setJump(null),
    busy: !search.settled || search.fetching,
    note: search.error || (search.data ? unreadWords(search.data) : ""),
    marked: search.query,
  };
}
