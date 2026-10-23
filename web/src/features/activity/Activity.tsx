// The activity panel behind the bell: what happened across the agents while nobody was looking — every alert the
// pushes carry, by day, newest first — a row opens its pane; the alert settings wait at the foot.

import { useEffect } from "react";
import { useAllPanes } from "@/app/current";
import { actions } from "@/app/store";
import { useRoster } from "@/features/roster/useRoster";
import { usePref } from "@/lib/prefs";
import { useQuery } from "@/lib/query";
import { whenShort } from "@/lib/time";
import { Dialog, Icon, Logo, Notice, type IconName } from "@/ui";
import { fetchFeed, type Entry } from "./api";
import { groupByDay } from "./group";

const KIND_ICONS: Record<string, IconName> = { input: "alert-triangle", done: "circle-check", stuck: "alert-triangle", waiting: "alert-triangle", loop: "refresh-cw", schedule: "calendar", task: "list", test: "bell" };

export default function Activity({ onClose }: { onClose: () => void }) {
  const { machines, feed } = useRoster();
  const panes = useAllPanes();
  const { data: entries, error } = useQuery<Entry[]>(() => fetchFeed(), [], { every: 10_000 });
  const [, setSeen] = usePref<number>("feed.seen", 0);
  useEffect(() => {
    if (feed) setSeen(feed);
  }, [feed, setSeen]);
  const open = (entry: Entry) => {
    if (!entry.pane) return;
    actions.selectPane(entry.machine, entry.pane);
    actions.setMode("herdr");
    actions.setLens("chat");
    onClose();
  };
  const where = (entry: Entry) => {
    const found = panes.find((candidate) => candidate.machine.id === entry.machine && candidate.pane.id === entry.pane);
    const machine = machines.find((candidate) => candidate.id === entry.machine);
    return { agent: found?.pane.agent ?? "", gone: !!entry.pane && !found, machine: entry.machine !== "local" ? machine?.label ?? entry.machine : "" };
  };
  return (
    <Dialog title="Activity" icon="bell" size="md" className="activity-dialog" onClose={onClose}>
      {error && <Notice tone="error">{error}</Notice>}
      {entries && entries.length === 0 && <Notice tone="hint">Nothing yet: a finish, a question, a watchdog note or a task's move lands here as it happens.</Notice>}
      {entries &&
        groupByDay(entries).map((day) => (
          <section key={day.label} className="activity-day">
            <h3 className="activity-day-head">{day.label}</h3>
            <ul className="activity-list">
              {day.entries.map((entry) => {
                const at = where(entry);
                return (
                  <li key={entry.id}>
                    <button type="button" className={`activity-row${at.gone ? " gone" : ""}`} disabled={!entry.pane} onClick={() => open(entry)} title={at.gone ? "That pane is gone" : entry.pane ? "Open the pane" : undefined}>
                      <span className="activity-when">{whenShort(entry.at)}</span>
                      {at.agent ? <Logo agent={at.agent} /> : <Icon name={KIND_ICONS[entry.kind] ?? "bell"} size={14} />}
                      <span className="activity-text">
                        <span className="activity-title">
                          {entry.title}
                          {at.machine && <span className="activity-machine"> · {at.machine}</span>}
                        </span>
                        {entry.body && <span className="activity-body">{entry.body}</span>}
                      </span>
                    </button>
                  </li>
                );
              })}
            </ul>
          </section>
        ))}
      <div className="modal-actions activity-foot">
        <button type="button" className="btn" onClick={() => actions.openPanel("alerts")}>
          <Icon name="settings" size={14} /> Alerts on this device
        </button>
      </div>
    </Dialog>
  );
}
