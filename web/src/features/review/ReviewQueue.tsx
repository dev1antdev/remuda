// The Review mode: every workspace with uncommitted work in one list — the ones rejected whose agent says it is ready
// again first, then the ones with changes since you looked — and the Changes lens of the picked one beside it:
// approve, reject, comment from here instead of visiting each pane.

import { useEffect, useMemo, useState } from "react";
import { actions, app } from "@/app/store";
import { useRoster } from "@/features/roster/useRoster";
import { basename, plural } from "@/lib/text";
import { Icon, Logo, PANELS, Resizer } from "@/ui";
import { reviewQueue, type QueueItem } from "./queue";
import Review from "./Review";
import { t } from "@/lib/i18n";

export default function ReviewQueue() {
  const { machines, refresh } = useRoster();
  const queue = useMemo(() => reviewQueue(machines), [machines]);
  const [picked, setPicked] = useState<string | null>(null);
  const asked = app.use((s) => s.reviewAsked);
  // a folder asked for from elsewhere (a task's run) is picked once the roster lists it; one without changes is said so
  useEffect(() => {
    if (!asked) return;
    const wanted = queue.find((candidate) => candidate.machine.id === asked.machineId && candidate.dir === asked.dir);
    if (wanted) setPicked(wanted.key);
    else actions.notify(t("Nothing to review in {folder} yet", { folder: basename(asked.dir) || asked.dir }), { brief: true });
    actions.reviewPicked();
  }, [asked, queue]);
  const item = queue.find((candidate) => candidate.key === picked) ?? null;
  const fresh = queue.filter((candidate) => candidate.fresh > 0).length;
  const ready = queue.filter((candidate) => candidate.ready).length;
  const manyMachines = new Set(queue.map((candidate) => candidate.machine.id)).size > 1;
  return (
    <main className={`queue${item ? " has-pick" : ""}`}>
      <aside className="queue-list" aria-label={t("Workspaces to review")}>
        <div className="queue-head">
          <span className="queue-count">{queue.length === 0 ? t("Nothing to review") : ready ? t("{workspaces} ready again", { workspaces: plural(ready, "workspace") }) : fresh ? t("{workspaces} with new changes", { workspaces: plural(fresh, "workspace") }) : t("{workspaces} with changes", { workspaces: plural(queue.length, "workspace") })}</span>
        </div>
        {queue.length === 0 && <p className="queue-empty">{t("Every workspace is clean or reviewed.")}</p>}
        {queue.map((candidate) => (
          <QueueRow key={candidate.key} item={candidate} on={candidate.key === picked} machine={manyMachines} onPick={() => setPicked(candidate.key)} />
        ))}
      </aside>
      <Resizer spec={PANELS.queue} edge="right" label={t("Review queue width")} />
      <section className="queue-main">
        {item ? (
          <>
            <div className="queue-main-head">
              <button type="button" className="btn sm queue-back" onClick={() => setPicked(null)}>
                <Icon name="chevron-left" size={14} /> {t("Queue")}
              </button>
              <span className="queue-title">
                {item.session.name}
                {item.machine.id !== "local" && <span className="queue-where"> · {item.machine.label}</span>}
              </span>
            </div>
            <Review key={item.key} machineId={item.machine.id} paneId={item.pane?.id ?? ""} dir={item.dir} agent={item.pane?.agent ?? ""} onSeen={refresh} active />
          </>
        ) : (
          <div className="placeholder">
            <p>{queue.length === 0 ? t("Nothing waits for a review.") : t("Pick a workspace to review its changes.")}</p>
          </div>
        )}
      </section>
    </main>
  );
}

function QueueRow({ item, on, machine, onPick }: { item: QueueItem; on: boolean; machine: boolean; onPick: () => void }) {
  const count = item.fresh || item.changed;
  const countWords = item.fresh ? t("{files} changed since you looked", { files: plural(item.fresh, "file") }) : item.changed ? t("{files} changed, all seen", { files: plural(item.changed, "file") }) : t("everything there is committed");
  return (
    <button type="button" className={`queue-item${on ? " on" : ""}${item.fresh ? " fresh" : ""}${item.ready ? " ready" : ""}`} onClick={onPick} title={item.ready ? `${item.dir}\n${t("Rejected at the last look; the agent says it is ready for another")}` : item.dir}>
      {item.pane?.agent ? <Logo agent={item.pane.agent} /> : <Icon name="terminal" size={14} />}
      <span className="queue-name">
        <span className="queue-title">
          {item.session.name}
          {machine && <span className="queue-where"> · {item.machine.label}</span>}
        </span>
        {item.ready && <span className="queue-sub">{t("says it is ready again")}</span>}
      </span>
      {item.ready ? (
        <span className="badge ready" title={countWords}>
          {t("ready again")}
        </span>
      ) : (
        <span className={`badge${item.fresh ? "" : " quiet"}`} title={countWords}>
          {count}
        </span>
      )}
    </button>
  );
}
