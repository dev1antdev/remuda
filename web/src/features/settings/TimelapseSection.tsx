// The Timelapse section of Settings: whether the agents' turns are recorded on the cockpit, how much that keeps, and
// Delete all.

import { useState } from "react";
import { actions } from "@/app/store";
import { deleteRecordings, fetchTimelapse, saveTimelapse } from "@/features/terminal/timelapse/api";
import { t, tn } from "@/lib/i18n";
import { useQuery } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { Icon, Notice } from "@/ui";
import { keptWords } from "./timelapse";

export default function TimelapseSection() {
  const { data: stats, mutate } = useQuery(fetchTimelapse, []);
  const [failure, setFailure] = useState("");
  if (!stats) return null;
  const flip = async (on: boolean) => {
    setFailure("");
    try {
      const saved = await saveTimelapse(on);
      mutate(() => saved);
    } catch (e) {
      setFailure(errMsg(e));
    }
  };
  const clear = () =>
    actions.ask(t("Delete every timelapse recording? The conversations stay as they are."), t("Delete all"), () =>
      void deleteRecordings()
        .then((done) => {
          mutate(() => done.stats);
          actions.notify(tn(done.deleted, "Deleted {n} recording", "Deleted {n} recordings"), { brief: true });
        })
        .catch((e) => setFailure(errMsg(e))),
    );
  return (
    <section className="settings-section timelapse-settings">
      <h3>{t("Timelapse")}</h3>
      <label className="settings-row">
        <span>
          {t("Record the agents' turns")}
          <small>
            {stats.scratch
              ? t("This remuda runs beside the real one (REMUDA_STATE_DIR is set) and records nothing: the real one does. Started with -\u2060record, it records too.")
              : t("While an agent works, the cockpit reads its screen every 4 s — with no page open too — and keeps the turn once it ends, if it lasted a minute. The recordings stay on this machine, out of the nightly backups.")}
          </small>
        </span>
        <input type="checkbox" checked={stats.on && !stats.scratch} disabled={stats.scratch} onChange={(e) => void flip(e.target.checked)} />
      </label>
      <div className="settings-row">
        <span>
          {t("Kept")}
          <small>{keptWords(stats)}</small>
        </span>
        <button type="button" className="btn sm danger" disabled={stats.count === 0} onClick={clear}>
          <Icon name="trash" size={14} /> {t("Delete all")}
        </button>
      </div>
      {failure && <Notice tone="error">{failure}</Notice>}
    </section>
  );
}
