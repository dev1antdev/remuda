// Undo an agent's turn in a folder: the turn (the newest by default), what the undo would do to each file as the folder
// stands now, the commits it made offered as reverts, and why it cannot when it cannot — the files that changed since
// named. Nothing is undone before the button; the agent is told nothing.

import { useEffect, useState } from "react";
import { actions } from "@/app/store";
import { useSharing } from "@/features/roster/useSharing";
import { t } from "@/lib/i18n";
import { useQuery } from "@/lib/query";
import { errMsg, plural } from "@/lib/text";
import { Dialog, Notice, Select } from "@/ui";
import { fetchPlan, fetchTurns, undoTurn, type UndoPlan } from "./api";
import { canRevert, defaultTurn, fileWords, nothingToDo, pushedWords, sentence, turnHint, turnLabel, undoneWords } from "./undo";

export default function UndoDialog({ machineId, dir, turn, onClose }: { machineId: string; dir: string; turn?: string; onClose: () => void }) {
  const { redact } = useSharing(); // under the screen-sharing mode a hidden folder's paths go by their neutral names
  const list = useQuery(() => fetchTurns(machineId, dir), [machineId, dir]);
  const turns = list.data?.turns ?? [];
  const [picked, setPicked] = useState<string | null>(turn ?? null);
  const chosen = picked ?? defaultTurn(turns)?.id ?? null;
  const planned = useQuery<UndoPlan | null>(() => (chosen ? fetchPlan(machineId, chosen) : Promise.resolve(null)), [machineId, chosen]);
  const plan = planned.data && planned.data.turn.id === chosen ? planned.data : null;
  const [revert, setRevert] = useState(true);
  useEffect(() => setRevert(true), [chosen]);
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState("");
  const reverting = revert && canRevert(plan);
  const commits = plan?.turn.commits ?? [];
  const run = async () => {
    if (!chosen) return;
    setBusy(true);
    setFailure("");
    try {
      const result = await undoTurn(machineId, chosen, reverting);
      actions.noteUndone();
      actions.notify(undoneWords(result), { brief: true });
      onClose();
    } catch (e) {
      setFailure(errMsg(e));
      actions.noteUndone(); // a revert may have gone through before git stopped: the lens reads the folder again
      planned.reload();
    } finally {
      setBusy(false);
    }
  };
  const error = failure || list.error || planned.error;
  return (
    <Dialog title={t("Undo a turn")} icon="undo" size="md" onClose={onClose} className="undo-dialog">
      <p className="modal-hint">{t("Puts the files back as they were when the agent's turn began. A commit it made is reverted with a new commit, so nothing is lost. The agent is told nothing.")}</p>
      {turns.length > 1 && chosen && (
        <label>
          {t("Turn")}
          <Select value={chosen} onChange={setPicked} options={turns.map((one) => ({ value: one.id, label: turnLabel(one), hint: turnHint(one) }))} />
        </label>
      )}
      {turns.length === 1 && <p className="modal-hint undo-turn-line">{[turnLabel(turns[0]!), turnHint(turns[0]!)].filter(Boolean).join(" · ")}</p>}
      {list.data && turns.length === 0 && (
        <Notice tone="hint">{list.data.recording ? t("No agent's turn has changed anything in this folder yet.") : t("This remuda keeps no turns: a second instance beside the real one leaves them to it.")}</Notice>
      )}
      {chosen && !plan && !planned.error && <Notice tone="hint">{t("Reading the folder…")}</Notice>}
      {plan?.refused && (
        <Notice tone="warn" icon="alert-triangle" className="undo-refused">
          {redact(sentence(plan.refused.why))}
          {plan.refused.files && plan.refused.files.length > 0 && (
            <ul className="undo-refused-files">
              {plan.refused.files.map((path) => (
                <li key={path}>
                  <bdi dir="ltr">{redact(path)}</bdi>
                </li>
              ))}
            </ul>
          )}
        </Notice>
      )}
      {plan && !plan.refused && (
        <>
          {plan.turn.shared && <Notice tone="warn">{t("Another agent worked in this folder during this turn: its changes are undone too.")}</Notice>}
          {(plan.turn.files?.length ?? 0) > 0 && (
            <ul className="pick-list undo-files">
              {(plan.turn.files ?? []).map((file) => (
                <li key={file.path} className="pick-row">
                  <span className="pick-name" title={redact(file.path)}>
                    <bdi dir="ltr">{redact(file.path)}</bdi>
                  </span>
                  <span className="pick-hint">
                    {fileWords(file, reverting)}
                    {file.binary ? ` · ${t("binary")}` : ""}
                  </span>
                </li>
              ))}
            </ul>
          )}
          {commits.length > 0 && (
            <div className="undo-commits">
              <label className="check-row undo-revert">
                <input type="checkbox" checked={reverting} disabled={!canRevert(plan) || busy} onChange={(e) => setRevert(e.target.checked)} />
                {t("Also revert the {commits} it made: git revert, a new commit each, newest first", { commits: plural(commits.length, "commit") })}
              </label>
              <ul className="undo-commit-list">
                {commits.map((commit) => (
                  <li key={commit.sha}>
                    <code>{commit.sha.slice(0, 8)}</code> {redact(commit.subject)}
                    {commit.merge ? ` · ${t("a merge")}` : ""}
                  </li>
                ))}
              </ul>
              {plan.revert_why && <Notice tone="hint">{redact(sentence(plan.revert_why))}</Notice>}
              {reverting && plan.pushed && plan.pushed.length > 0 && <Notice tone="warn">{pushedWords(plan.pushed)}</Notice>}
              {!reverting && plan.left_as_is && plan.left_as_is.length > 0 && <Notice tone="hint">{t("Without the reverts, {files} stay as the commits have them.", { files: plural(plan.left_as_is.length, "file") })}</Notice>}
            </div>
          )}
        </>
      )}
      {error && <Notice tone="error">{redact(sentence(error))}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          {t("Cancel")}
        </button>
        <button type="button" className="btn danger" disabled={busy || !plan || !!plan.refused || nothingToDo(plan, revert)} onClick={() => void run()}>
          {busy ? t("Undoing…") : t("Undo this turn")}
        </button>
      </div>
    </Dialog>
  );
}
