// The worktrees a machine still keeps under ~/.herdr/worktrees that no workspace has open — closed in herdr itself, or
// before a close took its worktree down — each with its branch and size or why it cannot go; the clean ones ticked,
// removed at the button as a close removes one (the folder, the branch once merged).

import { useEffect, useState } from "react";
import { act } from "@/app/act";
import { actions } from "@/app/store";
import { useQuery } from "@/lib/query";
import { errMsg, plural } from "@/lib/text";
import { Dialog, Notice } from "@/ui";
import { t } from "@/lib/i18n";
import { listLeftovers, removeLeftovers, type Machine } from "./api";
import { leftoverWords, removable, removedWords, sizeWords } from "./tidy";
import { useSharing } from "./useSharing";

export default function LeftoversDialog({ machine, onClose }: { machine: Machine; onClose: () => void }) {
  const { data, error, loading, reload } = useQuery(() => listLeftovers(machine.id), [machine.id]);
  const [ticked, setTicked] = useState<Set<string>>(new Set());
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState("");
  const list = data?.leftovers ?? [];
  useEffect(() => setTicked(new Set((data?.leftovers ?? []).filter(removable).map((left) => left.path))), [data]);
  const chosen = list.filter((left) => ticked.has(left.path) && removable(left));
  const toggle = (path: string) =>
    setTicked((before) => {
      const next = new Set(before);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      return next;
    });
  const remove = async () => {
    setBusy(true);
    setFailure("");
    try {
      const { outcomes } = await removeLeftovers(machine.id, chosen.map((left) => left.path));
      const said = removedWords(outcomes);
      actions.notify(said.text, { error: said.error });
      await act(Promise.resolve());
      reload();
    } catch (e) {
      setFailure(errMsg(e));
    } finally {
      setBusy(false);
    }
  };
  const freed = chosen.reduce((sum, left) => sum + left.kb, 0);
  const { machineLabel, redact } = useSharing(); // under the screen-sharing mode the machine and the folders by their neutral names
  return (
    <Dialog title={t("Leftover worktrees on {machine}", { machine: machineLabel(machine) })} icon="git-branch" size="md" onClose={onClose}>
      <p className="modal-hint">{t("Worktrees under {root} that no workspace has open. A clean one goes with its branch once merged; one with uncommitted changes stays.", { root: redact(data?.root ?? "~/.herdr/worktrees") })}</p>
      {(error || failure) && <Notice tone="error">{redact(failure || error || "")}</Notice>}
      {loading && !data && <Notice tone="hint">{t("Looking at the worktrees…")}</Notice>}
      {data && list.length === 0 && <Notice tone="success">{t("Nothing left behind: every worktree here has its workspace.")}</Notice>}
      {list.length > 0 && (
        <ul className="pick-list">
          {list.map((left) => (
            <li key={left.path}>
              <label className="pick-row" title={redact(left.path)}>
                <input type="checkbox" checked={ticked.has(left.path) && removable(left)} disabled={!removable(left) || busy} onChange={() => toggle(left.path)} />
                <span className="pick-name">
                  {redact(left.repo)} › {redact(left.name)}
                </span>
                <span className="pick-hint">{redact(leftoverWords(left))}</span>
              </label>
            </li>
          ))}
        </ul>
      )}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          {t("Close")}
        </button>
        <button type="button" className="btn danger" disabled={busy || chosen.length === 0} onClick={() => void remove()}>
          {busy ? t("Removing…") : chosen.length ? t("Remove {worktrees} · {size}", { worktrees: plural(chosen.length, "worktree"), size: sizeWords(freed) }) : t("Remove")}
        </button>
      </div>
    </Dialog>
  );
}
