// The two groups over the machines, after Orchi's and munza's sidebars: "Needs you" — every agent waiting on the person,
// the oldest wait first, each with what it asks (read off its screen) or the watchdog's note, a click opening it and
// nothing answered from here — and the agents the person pinned, in pin order. Neither is drawn when it holds nobody.

import type { KeyboardEvent } from "react";
import { actions } from "@/app/store";
import { Logo } from "@/ui";
import { askWords, type Pinned, type Waiting } from "./sidebarAgents";
import { AgentLine, type LineProps } from "./AgentLines";
import { folderOf, shortAge } from "./workspaces";
import type { Sharing } from "./useSharing";
import { t, tc } from "@/lib/i18n";

// NeedsYou is the inbox: a row per waiting agent, the ask in the colour of a question, a note in the watchdog's
export function NeedsYou({ waiting, questions, share, now, selected }: { waiting: Waiting[]; questions: Map<string, string>; share: Sharing; now: number; selected: string | null }) {
  if (waiting.length === 0) return null;
  return (
    <section className="sidebar-group needs-you" aria-label={t("Needs you")}>
      <div className="sidebar-group-head is-needs">
        <span>{t("Needs you")}</span>
        <span className="badge">{waiting.length}</span>
      </div>
      {waiting.map((w) => (
        <InboxRow key={w.key} waiting={w} asked={questions.get(w.key)} share={share} now={now} on={selected === w.key} />
      ))}
    </section>
  );
}

function InboxRow({ waiting: w, asked, share, now, on }: { waiting: Waiting; asked: string | undefined; share: Sharing; now: number; on: boolean }) {
  const hidden = share.sessionHidden(w.machine.id, w.session);
  const workspace = hidden ? share.folderName(w.machine.id, folderOf(w.session), w.session.name) : w.session.name;
  const machine = share.machineLabel(w.machine);
  const ask = hidden ? t("waiting for your answer") : askWords(w.pane, asked);
  const note = w.pane.radar?.state !== "blocked";
  const open = () => actions.openPane(w.machine.id, w.pane.id);
  const onKey = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.target !== e.currentTarget || (e.key !== "Enter" && e.key !== " ")) return;
    e.preventDefault();
    open();
  };
  return (
    <div className={`inbox-row${on ? " on" : ""}${note ? " is-note" : ""}`} role="button" tabIndex={0} onClick={open} onKeyDown={onKey} title={`${workspace} · ${machine}\n${ask}`}>
      <Logo agent={w.pane.agent} />
      <span className="inbox-text">
        <span className="inbox-where">
          <span className="inbox-workspace">{workspace}</span>
          <span className="inbox-machine">{machine}</span>
        </span>
        <span className="inbox-ask">{ask}</span>
      </span>
      <span className="ws-age">{shortAge(w.since, now)}</span>
    </div>
  );
}

// PinnedAgents is the pinned group: each agent's line, its workspace and machine quietly after its title
export function PinnedAgents({ pinned, share, ...props }: Omit<LineProps, "machine" | "hidden" | "where" | "selectedPane"> & { pinned: Pinned[]; share: Sharing; selected: string | null }) {
  if (pinned.length === 0) return null;
  const { selected, ...line } = props;
  return (
    <section className="sidebar-group pinned" aria-label={tc("sidebar", "Pinned")}>
      <div className="sidebar-group-head">
        <span>{tc("sidebar", "Pinned")}</span>
        <span className="badge quiet">{pinned.length}</span>
      </div>
      <div className="ws-agents">
        {pinned.map(({ key, machine, row }) => {
          const hidden = share.sessionHidden(machine.id, row.session);
          const workspace = hidden ? share.folderName(machine.id, folderOf(row.session), row.session.name) : row.session.name;
          return (
            <AgentLine
              key={key}
              row={row}
              machine={machine}
              hidden={hidden}
              where={`${workspace} · ${share.machineLabel(machine)}`}
              selectedPane={selected === key ? row.pane.id : null}
              {...line}
            />
          );
        })}
      </div>
    </section>
  );
}
