// Remove worktree, after Orca's: a table of what goes — the folder Deleted, the branch Kept until the person flips it —
// and in red what would be lost; Remove anyway runs git's --force past it. Another workspace on the checkout makes it
// Close workspace, the folder kept. An agent at work there holds it back. The server looks again on confirm and
// refuses what changed since this opened.

import { useEffect, useState } from "react";
import { actions } from "@/app/store";
import { useQuery } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { Dialog, Icon, Notice, Tip } from "@/ui";
import { t, tc } from "@/lib/i18n";
import type { Machine, Session } from "../api";
import { tidyWords } from "../tidy";
import { useSharing } from "../useSharing";
import { fetchRemoval, removeAsShown } from "./api";
import { blockedWords, branchGoesByDefault, buttonWords, rows, sharedHere, sharedWords, warnings } from "./removal";

export default function RemoveWorktreeDialog({ machine, session, onDone, onClose }: { machine: Machine; session: Session; onDone: () => void; onClose: () => void }) {
  const share = useSharing();
  const look = useQuery(() => fetchRemoval(machine.id, session.id), [machine.id, session.id]);
  const rm = look.data;
  const [deleteBranch, setDeleteBranch] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [changed, setChanged] = useState(false);
  useEffect(() => {
    if (rm) setDeleteBranch(branchGoesByDefault(rm));
  }, [rm?.stamp]); // eslint-disable-line react-hooks/exhaustive-deps

  const go = async (keepFolder: boolean) => {
    if (!rm) return;
    setBusy(true);
    setError("");
    try {
      const answer = await removeAsShown(machine.id, session.id, { delete_branch: deleteBranch, stamp: rm.stamp, keep_folder: keepFolder });
      if (answer.worktree) {
        const said = tidyWords(answer.worktree);
        actions.notify(said.text, { error: said.error });
      }
      onDone();
    } catch (e) {
      const why = errMsg(e);
      setBusy(false);
      if (/changed since this opened/.test(why)) {
        setChanged(true);
        look.reload();
        return;
      }
      setError(why);
    }
  };

  const shared = rm ? sharedHere(rm) : false;
  const blocked = rm ? blockedWords(rm) : "";
  const lost = rm ? warnings(rm, deleteBranch) : [];
  const title = shared ? t("Close workspace") : t("Remove worktree");
  const subtitle = [share.redact(session.name), share.redact(rm?.repo_name ?? session.worktree?.repo_name ?? "")].filter(Boolean).join(" · ");
  return (
    <Dialog
      as="form"
      size="md"
      className="remove-worktree"
      labelledBy="remove-worktree-title"
      onClose={onClose}
      onSubmit={(e) => {
        e.preventDefault();
        if (!busy && rm && !blocked) void go(false);
      }}
    >
      <div className="rw-head">
        <span className="rw-tile" aria-hidden="true">
          <Icon name="trash" size={18} />
        </span>
        <div className="rw-titles">
          <h2 id="remove-worktree-title">{title}</h2>
          <span className="rw-sub">{subtitle}</span>
        </div>
        <Tip text={t("Close")}>
          <button type="button" className="icon-btn" aria-label={t("Close")} onClick={onClose}>
            <Icon name="x" size={16} />
          </button>
        </Tip>
      </div>
      {look.loading && <p className="modal-hint">{t("Checking for unsaved work…")}</p>}
      {look.error && <Notice tone="error">{look.error}</Notice>}
      {rm && (
        <>
          <div className="rw-table">
            {rows(rm, deleteBranch, rm.home ?? "").map((row) => (
              <div key={row.what} className="rw-line">
                <Icon name={row.what === "folder" ? "folder" : "git-branch"} size={14} />
                <span className="rw-label">{row.label}</span>
                <span className="rw-value">
                  <span className={row.what === "branch" ? "mono" : undefined}>{share.redact(row.value)}</span>
                  {row.under && <small>{share.redact(row.under)}</small>}
                </span>
                {row.flips ? (
                  <Tip text={deleteBranch ? t("Click to keep it") : t("Click to delete it")}>
                    <button type="button" className={`btn sm rw-pill is-${row.pill.toLowerCase()}`} aria-pressed={deleteBranch} onClick={() => setDeleteBranch(!deleteBranch)}>
                      {tc("worktree-pill", row.pill)}
                    </button>
                  </Tip>
                ) : (
                  <span className={`rw-pill is-${row.pill.toLowerCase()}`}>{tc("worktree-pill", row.pill)}</span>
                )}
              </div>
            ))}
          </div>
          {shared && <p className="modal-hint">{share.redact(sharedWords(rm))}</p>}
          {lost.map((line) => (
            <Notice key={line} tone="error" icon="alert-triangle">
              {line}
            </Notice>
          ))}
          {blocked && <Notice tone="warn">{blocked}</Notice>}
          {changed && <Notice tone="warn">{t("It changed since this opened: look again, then press the button once more to go ahead.")}</Notice>}
        </>
      )}
      {error && <Notice tone="error">{error}</Notice>}
      <div className="modal-actions">
        <span className="rw-foot">{rm && !shared ? t("Its workspace closes too.") : ""}</span>
        <button type="button" className="btn" onClick={onClose}>
          {t("Cancel")}
        </button>
        {rm && !shared && lost.length > 0 && (
          <button type="button" className="btn" disabled={busy} onClick={() => void go(true)}>
            {t("Close, keep the folder")}
          </button>
        )}
        <button type="submit" className={`btn ${shared ? "primary" : "danger"}`} disabled={!rm || busy || !!blocked}>
          {rm ? buttonWords(rm, deleteBranch, busy) : t("Remove")}
        </button>
      </div>
    </Dialog>
  );
}
