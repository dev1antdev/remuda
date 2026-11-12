// useQuestions reads, for the creatures that ask the person something, what they ask — the dialog off the pane's screen
// (`GET /api/question`), one pane after another every QUESTIONS_EVERY while the page is in view, at most MAX_ASKING of
// them — so each holds it up in its bubble. Nothing is read for a zoo where nobody asks.

import { useEffect, useState } from "react";
import { fetchQuestion } from "@/features/chat/api";
import { questionBubble } from "@/features/roster/zooLife";
import { pageVisible } from "@/lib/visibility";

export const QUESTIONS_EVERY = 10_000;
export const MAX_ASKING = 8;

export type Asker = { key: string; machine: string; pane: string };

export function useQuestions(askers: Asker[]): Map<string, string> {
  const [held, setHeld] = useState<Map<string, string>>(() => new Map());
  const wanted = askers.slice(0, MAX_ASKING);
  const signature = wanted.map((a) => a.key).join(" ");
  useEffect(() => {
    if (!signature) {
      setHeld((was) => (was.size ? new Map() : was));
      return;
    }
    let stopped = false;
    const read = async () => {
      if (!pageVisible()) return;
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
      setHeld(next);
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
