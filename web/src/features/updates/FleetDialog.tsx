// Fleet setup: how every machine is set up, side by side — the agents and how they were installed, herdr and its hooks
// and plugins, remuda and its MCP server and status line, Claude Code's settings, the global instruction files, and every
// repository checked out on two machines or more. The value most machines share is the norm and one that differs is
// marked; an update remuda already has brings it in line through the Updates dialog, the command shown before Run, and
// the rest is said as what to do by hand: remuda never writes the agents' configs beyond its own updates.

import { useState } from "react";
import type { Machine } from "@/features/roster/api";
import { useRoster } from "@/features/roster/useRoster";
import { useSharing } from "@/features/roster/useSharing";
import { useQuery } from "@/lib/query";
import { ago } from "@/lib/time";
import { t } from "@/lib/i18n";
import { Dialog, Icon, Notice } from "@/ui";
import { fleetSetup, type FleetSection, type FleetTable } from "./api";
import { bringInLine, byHand, sectionTitle, shownSections, summaryWords } from "./fleet";
import FleetGrid from "./FleetGrid";
import UpdatesDialog from "./Updates";

export default function FleetDialog({ onClose }: { onClose: () => void }) {
  const { machines } = useRoster();
  const sharing = useSharing(); // under the screen-sharing mode the machines and the repositories by their neutral names
  const [reads, setReads] = useState(0); // a Read again, or an update run from here: every machine read at once
  const { data: table, error, loading, fetching } = useQuery(() => fleetSetup(reads > 0), [reads], { keep: true });
  const [all, setAll] = useState(false);
  const [bringing, setBringing] = useState<{ machine: Machine; what: string } | null>(null);
  if (bringing)
    return <UpdatesDialog machine={bringing.machine} what={bringing.what} onClose={() => setBringing(null)} onDone={() => setReads((n) => n + 1)} />;
  const rosterMachine = (id: string) => machines.find((m) => m.id === id);
  const name = (id: string) => {
    const known = rosterMachine(id);
    if (known) return sharing.machineLabel(known).split(" · ")[0]!;
    return sharing.redact(table?.machines.find((m) => m.id === id)?.label ?? id).split(" · ")[0]!;
  };
  const title = (section: FleetSection) => {
    if (section.id !== "repo") return sectionTitle(section);
    const hidden = Object.entries(section.folders ?? {}).find(([machine, folder]) => sharing.folderHidden(machine, folder));
    return hidden ? sharing.folderName(hidden[0], hidden[1], section.name ?? "") : sharing.redact(section.name ?? "");
  };
  return (
    <Dialog title={t("Fleet setup")} icon="columns" size="lg" onClose={onClose} className="updates fleet-setup">
      {error && <Notice tone="error">{sharing.redact(error)}</Notice>}
      {!table ? (
        loading && <Notice tone="hint">{t("Reading every machine…")}</Notice>
      ) : (
        <FleetBody table={table} all={all} name={name} title={title} redact={sharing.redact} onBring={(machine, what) => {
          const known = rosterMachine(machine);
          if (known) setBringing({ machine: known, what });
        }} canBring={(machine) => !!rosterMachine(machine)} />
      )}
      <div className="modal-actions">
        {table && (
          <button type="button" className={`btn${all ? " on" : ""}`} aria-pressed={all} onClick={() => setAll((a) => !a)}>
            {t("Show every fact")}
          </button>
        )}
        <button type="button" className="btn" disabled={fetching} onClick={() => setReads((n) => n + 1)}>
          <Icon name="refresh-cw" size={14} className={fetching ? "spin" : undefined} /> {t("Read again")}
        </button>
        <button type="button" className="btn primary" onClick={onClose}>
          {t("Close")}
        </button>
      </div>
    </Dialog>
  );
}

function FleetBody({ table, all, name, title, redact, onBring, canBring }: {
  table: FleetTable;
  all: boolean;
  name: (machine: string) => string;
  title: (section: FleetSection) => string;
  redact: (text: string) => string;
  onBring: (machine: string, what: string) => void;
  canBring: (machine: string) => boolean;
}) {
  const sections = shownSections(table, all);
  const bring = bringInLine(table);
  const hand = byHand(table, name);
  const single = table.single ?? [];
  return (
    <>
      <p className="modal-hint">
        {t("Every machine side by side, read {when}: the value most of them share is the norm, and one that differs is marked.", { when: ago(table.at) })} {summaryWords(table)}
      </p>
      {sections.length > 0 && <FleetGrid table={table} sections={sections} name={name} title={title} redact={redact} />}
      {bring.length > 0 && (
        <section className="fleet-fixes">
          <h3>{t("Bring in line")}</h3>
          <ul className="everywhere-steps">
            {bring.map((b) => (
              <li key={`${b.machine}|${b.action.what}`} className="fleet-fix">
                <div className="fleet-fix-head">
                  <span className="fleet-fix-title">
                    <strong>{name(b.machine)}</strong> · {redact(b.action.title)}
                  </span>
                  <button type="button" className="btn sm" disabled={!canBring(b.machine)} onClick={() => onBring(b.machine, b.action.what)}>
                    {t("Run…")}
                  </button>
                </div>
                <span className="dim">{redact(b.rows.join(", "))}</span>
                <details>
                  <summary>{t("The command")}</summary>
                  <code className="updates-cmd">{redact(b.action.command)}</code>
                </details>
              </li>
            ))}
          </ul>
        </section>
      )}
      {hand.length > 0 && (
        <section className="fleet-fixes">
          <h3>{t("By hand")}</h3>
          <ul className="fleet-hand">
            {hand.map((h) => (
              <li key={h.key}>
                <strong>{redact(h.title)}</strong> <span>{redact(h.words)}</span>
              </li>
            ))}
          </ul>
        </section>
      )}
      {single.length > 0 && (
        <p className="modal-hint">
          {t("On one machine only, so nothing to compare: {list}.", {
            list: single.map((s) => `${title({ id: "repo", name: s.name, folders: { [s.machine]: s.folder }, rows: [] })} (${name(s.machine)})`).join(", "),
          })}
        </p>
      )}
    </>
  );
}
