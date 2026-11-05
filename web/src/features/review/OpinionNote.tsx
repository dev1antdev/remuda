// One comment of the second opinion: under its diff row, or in a list of its own when no row carries it; added to
// the person's review or dismissed from here.

import { Icon, Tip } from "@/ui";
import { t } from "@/lib/i18n";
import { severityWords, type OpinionComment } from "./opinion";

export function OpinionNote({ comment, reviewer, where, onAdopt, onDismiss }: {
  comment: OpinionComment;
  reviewer: string;
  where?: string; // the comment's address, for one listed apart from its row
  onAdopt: (comment: OpinionComment) => void;
  onDismiss: (id: number) => void;
}) {
  return (
    <div className={`review-opinion${comment.severity ? ` sev-${comment.severity}` : ""}`}>
      <span className="review-opinion-text">
        <span className="review-opinion-who">
          {reviewer}
          {comment.severity && <span className="review-opinion-sev">{severityWords(comment.severity)}</span>}
          {where && <span className="review-note-where">{where}</span>}
        </span>
        {comment.text}
      </span>
      <button type="button" className="btn sm" onClick={() => onAdopt(comment)}>
        <Icon name="plus" size={12} /> {t("Add to my review")}
      </button>
      <Tip text={t("Dismiss this comment")}>
        <button type="button" className="icon-btn sm" aria-label={t("Dismiss this comment")} onClick={() => onDismiss(comment.id)}>
          <Icon name="x" size={12} />
        </button>
      </Tip>
    </div>
  );
}
