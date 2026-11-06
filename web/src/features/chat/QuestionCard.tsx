// QuestionCard is what the agent asks, read off its screen while it waits: the dialog's words and a button per choice —
// a click walks the cursor there and presses Enter — or the blind keys: when the screen shows nothing to read, and under
// a multi-select's words, where Enter ticks a box and the person goes down to Submit.

import { useEffect, useState, type ReactNode } from "react";
import { useLive, useLiveConnected, watchPane } from "@/lib/live";
import { useQuery } from "@/lib/query";
import { fetchQuestion } from "./api";
import { PromptKeys } from "./PromptKeys";
import { choosable, keysTo, pressInTurn, questionLines, sameDialog } from "./question";
import { t } from "@/lib/i18n";

export function QuestionCard({ machineId, paneId, active, compact = false, onKeys, children }: {
  machineId: string;
  paneId: string;
  active: boolean; // read every 2 s only while on screen (10 s with the event socket up, and at once when the pane's status moves)
  compact?: boolean; // a tile: the head and the ask, the detail left out
  onKeys: (keys: string[]) => Promise<boolean | void> | boolean | void; // false: the keys did not reach the pane
  children: ReactNode; // the words before the blind keys when the screen shows no dialog
}) {
  const connected = useLiveConnected();
  const { data: asked, loading, reload } = useQuery(() => fetchQuestion(machineId, paneId), [machineId, paneId], { every: connected ? 10000 : 2000, enabled: active, keep: true });
  useEffect(() => {
    if (!active) return;
    return watchPane(machineId, paneId);
  }, [active, machineId, paneId]);
  useLive("chat", (event) => {
    if (active && event.machine === machineId && event.pane === paneId) reload();
  });
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
  if (!choosable(asked)) {
    return (
      <>
        <div className="question" title={asked.text}>
          {head && <div className="question-head">{head}</div>}
          {ask && ask !== head && <div className="question-ask">{ask}</div>}
          <div className="question-ask">{t("Several choices: Enter ticks one, then go down to Submit")}</div>
        </div>
        <PromptKeys onKeys={onKeys} />
      </>
    );
  }
  const choose = async (index: number) => {
    setSending(true);
    try {
      // the screen read again first: the cursor may have moved, or another dialog taken this one's place, since the card
      // read it; the keys walk from where the cursor is now, and nothing goes to a dialog the card did not show
      const now = await fetchQuestion(machineId, paneId).catch(() => null);
      if (now && choosable(now) && sameDialog(asked, now)) await pressInTurn(keysTo(now.options, index), (key) => onKeys([key]));
    } finally {
      setSending(false);
      setTimeout(reload, 600); // the screen after the answer: the next question, or none
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
        <button type="button" className="btn sm question-esc" disabled={sending} title={t("Escape: close the dialog")} onClick={() => void onKeys(["esc"])}>
          {t("Esc")}
        </button>
      </div>
    </div>
  );
}
