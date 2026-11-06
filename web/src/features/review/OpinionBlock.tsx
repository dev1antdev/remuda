// The second opinion's block at the top of the Changes lens: who reviewed, what it found in a sentence, the comments
// on files the lens does not list, and Add all / Dismiss all for every open comment. The comments themselves sit under
// their diff rows (Hunk), or under their file's diff when no row carries them any more (Diff).

import { actions } from "@/app/store";
import { Icon, Logo } from "@/ui";
import { t } from "@/lib/i18n";
import { addressOf, headWords, openComments, type Opinion, type OpinionComment } from "./opinion";
import { OpinionNote } from "./OpinionNote";

export function OpinionBlock({ opinion, reviewer, asking, apart, moved, onAdopt, onAdoptAll, onDismiss, onDismissAll }: {
  opinion: Opinion;
  reviewer: string;
  asking: boolean; // the reviewer waits for the person in its tab
  apart: OpinionComment[]; // on files the lens does not list
  moved: OpinionComment[]; // on files changed since it was asked: the lines it named may have moved
  onAdopt: (comment: OpinionComment) => void;
  onAdoptAll: () => void;
  onDismiss: (id: number) => void;
  onDismissAll: () => void;
}) {
  const open = openComments(opinion).length;
  const running = opinion.state === "running";
  const tabOpen = running || opinion.state === "unreadable" || !opinion.close;
  return (
    <section className={`opinion-block${running ? " running" : ""}`} aria-label={t("Second opinion")}>
      <div className="opinion-head">
        <Logo agent={opinion.agent} />
        <span className="opinion-title">{t("Second opinion · {agent}", { agent: reviewer })}</span>
        <span className="opinion-words">{headWords(opinion, reviewer, asking)}</span>
        {tabOpen && opinion.pane && (
          <button type="button" className="linkish" onClick={() => actions.selectPane(opinion.machine, opinion.pane)}>
            {t("Open its tab")}
          </button>
        )}
        {open > 0 && (
          <span className="opinion-actions">
            <button type="button" className="btn sm" onClick={onAdoptAll}>
              <Icon name="plus" size={12} /> {t("Add all")}
            </button>
            <button type="button" className="btn sm ghost" onClick={onDismissAll}>
              {t("Dismiss all")}
            </button>
          </span>
        )}
      </div>
      {opinion.summary && opinion.state === "done" && <p className="opinion-summary">{opinion.summary}</p>}
      {apart.length > 0 && (
        <div className="opinion-apart">
          <span className="modal-hint">{t("On files this list does not show:")}</span>
          {apart.map((comment) => (
            <OpinionNote key={comment.id} comment={comment} reviewer={reviewer} where={addressOf(comment)} onAdopt={onAdopt} onDismiss={onDismiss} />
          ))}
        </div>
      )}
      {moved.length > 0 && (
        <div className="opinion-apart opinion-moved">
          <span className="modal-hint">{t("On files changed since it was asked — the lines it named may have moved:")}</span>
          {moved.map((comment) => (
            <OpinionNote key={comment.id} comment={comment} reviewer={reviewer} where={addressOf(comment)} onAdopt={onAdopt} onDismiss={onDismiss} />
          ))}
        </div>
      )}
    </section>
  );
}
