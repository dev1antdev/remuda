// The Review mode: every workspace with uncommitted work in one list, the ones with changes since you looked first,
// and the Changes lens of the picked one beside it — approve, reject, comment from here instead of visiting each pane.

import { useEffect, useMemo, useState } from "react";
import { actions, app } from "@/app/store";
import { useRoster } from "@/features/roster/useRoster";
import { basename, plural } from "@/lib/text";
import { Icon, Logo, PANELS, Resizer } from "@/ui";
import { reviewQueue, type QueueItem } from "./queue";
import Review from "./Review";

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
    else actions.notify(`Nothing to review in ${basename(asked.dir) || asked.dir} yet`, { brief: true });
    actions.reviewPicked();
  }, [asked, queue]);
  const item = queue.find((candidate) => candidate.key === picked) ?? null;
  const fresh = queue.filter((candidate) => candidate.fresh > 0).length;
  const manyMachines = new Set(queue.map((candidate) => candidate.machine.id)).size > 1;
  return (
    <main className={`queue${item ? " has-pick" : ""}`}>
      <aside className="queue-list" aria-label="Workspaces to review">
        <div className="queue-head">
          <span className="queue-count">{queue.length === 0 ? "Nothing to review" : fresh ? `${plural(fresh, "workspace")} with new changes` : `${plural(queue.length, "workspace")} with changes`}</span>
        </div>
        {queue.length === 0 && <p className="queue-empty">Every workspace is clean or reviewed.</p>}
        {queue.map((candidate) => (
          <QueueRow key={candidate.key} item={candidate} on={candidate.key === picked} machine={manyMachines} onPick={() => setPicked(candidate.key)} />
        ))}
      </aside>
      <Resizer spec={PANELS.queue} edge="right" label="Review queue width" />
      <section className="queue-main">
        {item ? (
          <>
            <div className="queue-main-head">
              <button type="button" className="btn sm queue-back" onClick={() => setPicked(null)}>
                <Icon name="chevron-left" size={14} /> Queue
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
            <p>{queue.length === 0 ? "Nothing waits for a review." : "Pick a workspace to review its changes."}</p>
          </div>
        )}
      </section>
    </main>
  );
}

function QueueRow({ item, on, machine, onPick }: { item: QueueItem; on: boolean; machine: boolean; onPick: () => void }) {
  return (
    <button type="button" className={`queue-item${on ? " on" : ""}${item.fresh ? " fresh" : ""}`} onClick={onPick} title={item.dir}>
      {item.pane?.agent ? <Logo agent={item.pane.agent} /> : <Icon name="terminal" size={14} />}
      <span className="queue-name">
        {item.session.name}
        {machine && <span className="queue-where"> · {item.machine.label}</span>}
      </span>
      <span className={`badge${item.fresh ? "" : " quiet"}`} title={item.fresh ? `${plural(item.fresh, "file")} changed since you looked` : `${plural(item.changed, "file")} changed, all seen`}>
        {item.fresh || item.changed}
      </span>
    </button>
  );
}
