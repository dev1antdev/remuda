// The player's find: a field searching what stood on the screens of the recordings listed — this pane's and its
// folder's — "3 of 12", and the steps either way, each taking the player to the frame the text appeared in.

import { type KeyboardEvent } from "react";
import { t } from "@/lib/i18n";
import { Icon, Tip } from "@/ui";
import { stepWords } from "./found";
import { MIN_MARKED } from "./mark";

type Props = {
  text: string;
  onText: (text: string) => void;
  at: number; // the step on screen, -1 for none
  total: number;
  busy: boolean; // the typing has not settled, or the answer is on its way
  note: string; // what the search left unread, or why it failed
  onStep: (by: 1 | -1) => void;
};

export function ScreenFind({ text, onText, at, total, busy, note, onStep }: Props) {
  const asking = [...text.trim()].length >= MIN_MARKED;
  const onKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key !== "Enter") return;
    e.preventDefault();
    if (total > 0) onStep(e.shiftKey ? -1 : 1);
  };
  return (
    <>
      <div className="timelapse-find" role="search">
        <Icon name="search" size={14} className="timelapse-find-icon" />
        <input
          className="sm"
          type="text"
          value={text}
          placeholder={t("Find on these screens")}
          aria-label={t("Find on the recorded screens")}
          autoComplete="off"
          spellCheck={false}
          onChange={(e) => onText(e.target.value)}
          onKeyDown={onKey}
        />
        <span className="find-count" aria-live="polite">
          {!asking ? "" : busy ? "…" : stepWords(at, total)}
        </span>
        <Tip text={t("Previous match (Shift+Enter)")}>
          <button type="button" className="icon-btn sm" aria-label={t("Previous match")} disabled={total === 0 || busy} onClick={() => onStep(-1)}>
            <Icon name="chevron-up" size={14} />
          </button>
        </Tip>
        <Tip text={t("Next match (Enter)")}>
          <button type="button" className="icon-btn sm" aria-label={t("Next match")} disabled={total === 0 || busy} onClick={() => onStep(1)}>
            <Icon name="chevron-down" size={14} />
          </button>
        </Tip>
      </div>
      {asking && note && <div className="timelapse-find-note">{note}</div>}
    </>
  );
}
