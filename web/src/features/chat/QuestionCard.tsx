// QuestionCard is what the agent asks, read off its screen while it waits: the dialog's words and a button per choice —
// a click walks the cursor there and presses Enter — or, when the screen shows nothing to read, the blind keys.

import { useState, type ReactNode } from "react";
import { useQuery } from "@/lib/query";
import { fetchQuestion } from "./api";
import { PromptKeys } from "./PromptKeys";
import { keysTo, questionLines } from "./question";

export function QuestionCard({ machineId, paneId, active, compact = false, onKeys, children }: {
  machineId: string;
  paneId: string;
  active: boolean; // read every 2 s only while on screen
  compact?: boolean; // a tile: the head and the ask, the detail left out
  onKeys: (keys: string[]) => Promise<void> | void;
  children: ReactNode; // the words before the blind keys when the screen shows no dialog
}) {
  const { data: asked, loading } = useQuery(() => fetchQuestion(machineId, paneId), [machineId, paneId], { every: 2000, enabled: active, keep: true });
  const [sending, setSending] = useState(false);
  if (!asked) {
    return (
      <>
        {children}
        {!loading && <PromptKeys onKeys={onKeys} />}
      </>
    );
  }
  const { head, detail, ask } = questionLines(asked.text);
  const choose = async (index: number) => {
    setSending(true);
    try {
      for (const key of keysTo(asked.options, index)) await onKeys([key]);
    } finally {
      setSending(false);
    }
  };
  return (
    <div className="question" title={asked.text}>
      {head && <div className="question-head">{head}</div>}
      {!compact && detail && <pre className="question-detail">{detail}</pre>}
      {ask && ask !== head && <div className="question-ask">{ask}</div>}
      <div className="question-options">
        {asked.options.map((o, i) => (
          <button key={o.n} type="button" className={`btn sm${o.picked ? " on" : ""}`} disabled={sending} title={o.picked ? `${o.label} (the cursor is here)` : o.label} onClick={() => void choose(i)}>
            {o.label}
          </button>
        ))}
        <button type="button" className="btn sm question-esc" disabled={sending} title="Escape: close the dialog" onClick={() => void onKeys(["esc"])}>
          Esc
        </button>
      </div>
    </div>
  );
}
