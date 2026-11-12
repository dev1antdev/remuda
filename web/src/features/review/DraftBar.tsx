// The bar at the foot of the Changes lens: the draft's comments with Clear and Send, and the two verdicts.

import { plural } from "@/lib/text";
import { Icon, Tip } from "@/ui";
import type { Verdict } from "./api";
import { t } from "@/lib/i18n";

export function DraftBar({ comments, agent, otherAgents, sending, onClear, onSend, onPick, onVerdict }: {
  comments: number;
  agent: string; // the pane's agent, "" for a shell pane
  otherAgents: boolean; // agents besides this pane's to send the comments to
  sending: boolean;
  onClear: () => void;
  onSend: () => void; // to the pane's agent, with no verdict
  onPick: () => void; // to an agent picked in a dialog
  onVerdict: (verdict: Verdict) => void;
}) {
  return (
    <div className="review-draft-bar">
      <span className="review-draft-count">
        {comments > 0 && (
          <>
            <Icon name="message" size={14} /> {plural(comments, "comment")}
          </>
        )}
      </span>
      {comments > 0 && (
        <>
          <button type="button" className="linkish" onClick={onClear}>
            {t("Clear")}
          </button>
          {agent ? (
            <>
              <button type="button" className="btn" disabled={sending} title={t("Send the comments to {agent}, with no verdict", { agent })} onClick={onSend}>
                <Icon name="send" size={12} /> {t("Send")}
              </button>
              {otherAgents && (
                <Tip text={t("Send the comments to another agent")}>
                  <button type="button" className="icon-btn" aria-label={t("Send the comments to another agent")} disabled={sending} onClick={onPick}>
                    <Icon name="chevron-down" size={14} />
                  </button>
                </Tip>
              )}
            </>
          ) : (
            <button type="button" className="btn" disabled={sending} title={t("Send the comments to an agent, with no verdict")} onClick={onPick}>
              <Icon name="send" size={12} /> {t("Send…")}
            </button>
          )}
        </>
      )}
      <span className="review-verdicts">
        <button type="button" className="btn" onClick={() => onVerdict("changes")} title={t("Tell the agent what to change: the comments and a note go to it, and the next look starts from now")}>
          <Icon name="x" size={12} /> {t("Reject")}
        </button>
        <button type="button" className="btn primary" onClick={() => onVerdict("approved")} title={t("Tell the agent the changes are reviewed and it can carry on; the next look starts from now")}>
          <Icon name="check" size={12} /> {t("Approve")}
        </button>
      </span>
    </div>
  );
}
