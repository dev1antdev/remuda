// The confirm a chosen action opens under the pull request's head, on any tab: its question, in red what still stands in
// the way of a merge (GitHub merges past whatever it does not require), a squash's commit message to edit, Cancel and
// the action itself. A merge names the head commit on screen when the card opened, so a push since is refused with the
// sentence that says so; one the panel's own read brings while the card is open holds the merge back until it is seen.

import { useState } from "react";
import { actions as app } from "@/app/store";
import { errMsg } from "@/lib/text";
import { t } from "@/lib/i18n";
import { Notice } from "@/ui";
import { changeState, type Detail } from "./api";
import { actionDone, actionLabel, actionQuestion, blockers, headMoved, merges, splitMessage, squashMessage, type Action } from "./model";

export default function ConfirmCard({ machineId, pr, action, onDone, onCancel }: { machineId: string; pr: Detail; action: Action; onDone: () => void; onCancel: () => void }) {
  const [message, setMessage] = useState(() => squashMessage(pr));
  const [seen] = useState(() => ({ head: pr.head_oid, id: pr.id })); // the head the person saw when they chose it
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const inTheWay = merges(action) ? blockers(pr).map((b) => b.label) : [];
  const squash = action === "squash";
  const moved = headMoved(action, seen.head, pr.head_oid);
  const run = async () => {
    setBusy(true);
    setError("");
    try {
      const { subject, body } = splitMessage(message);
      const method = pr.merge_methods[0] ?? "merge";
      await changeState({ machine: machineId, repo: pr.repository, number: pr.number }, { action, head: seen.head, id: seen.id, method, ...(squash ? { subject, body } : {}) });
      app.notify(actionDone(action));
      onDone();
    } catch (e) {
      setError(t("{error} If it may have gone through, read GitHub again before trying once more.", { error: errMsg(e) }));
    } finally {
      setBusy(false);
    }
  };
  const danger = action === "close";
  return (
    <div className="pr-confirm card" role="group" aria-label={actionLabel(action)}>
      <p>{actionQuestion(action, pr)}</p>
      {inTheWay.length > 0 && <p className="pr-in-the-way">{t("Still in the way: {list}.", { list: inTheWay.join(", ") })}</p>}
      {squash && (
        <label>
          {t("Commit message")}
          <textarea rows={5} value={message} onChange={(e) => setMessage(e.target.value)} />
        </label>
      )}
      {merges(action) && pr.delete_branch_on_merge && <p className="pr-delete-branch">{t("GitHub deletes this branch when the pull request merges.")}</p>}
      {moved && <Notice tone="warn">{t("A newer push came in since you opened this: cancel, look at it, then choose again.")}</Notice>}
      {error && <Notice tone="error">{error}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" disabled={busy} onClick={onCancel}>
          {t("Cancel")}
        </button>
        <button type="button" className={`btn ${danger ? "danger" : "primary"}`} disabled={busy || moved || (squash && !message.trim())} onClick={() => void run()}>
          {busy ? t("Working…") : actionLabel(action)}
        </button>
      </div>
    </div>
  );
}
