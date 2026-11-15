// useQuestions reads, for the agents that ask the person something, what they ask — the dialog off the pane's screen
// (`GET /api/question`), one pane after another every QUESTIONS_EVERY while this very document is on screen (not while
// the Mac app's window is shut or the tab hides behind the pop-out, which keep the page counting as visible), at most
// MAX_ASKING of them, a round never begun while the last is still out — for the zoo's bubbles and the sidebar's "Needs
// you". Nothing is read while nobody asks (a caller off screen passes none), and a reading that says what the last one
// did draws nothing again.

import { useEffect, useState } from "react";
import { fetchQuestion } from "@/features/chat/api";
import { documentVisible } from "@/lib/visibility";
import { QUESTIONS_EVERY, askedOf, questionBubble, sameQuestions, type Asker } from "./questions";

export function useQuestions(askers: Asker[]): Map<string, string> {
  const [held, setHeld] = useState<Map<string, string>>(() => new Map());
  const { wanted, signature } = askedOf(askers);
  useEffect(() => {
    if (!signature) {
      setHeld((was) => (was.size ? new Map() : was));
      return;
    }
    let stopped = false;
    let reading = false;
    const read = async () => {
      if (reading || !documentVisible()) return;
      reading = true;
      const next = new Map<string, string>();
      for (const asker of wanted) {
        try {
          const text = questionBubble(await fetchQuestion(asker.machine, asker.pane));
          if (text) next.set(asker.key, text);
        } catch {
          // a pane that cannot be read holds nothing up
        }
        if (stopped) return;
      }
      reading = false;
      setHeld((was) => (sameQuestions(was, next) ? was : next));
    };
    void read();
    const id = setInterval(() => void read(), QUESTIONS_EVERY);
    return () => {
      stopped = true;
      clearInterval(id);
    };
  }, [signature]); // read again only when who asks changes
  return held;
}
