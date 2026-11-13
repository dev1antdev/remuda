// The live ports in the status bar, after Orca's: "3", the ports the pane's machine's workspaces listen on as the
// roster carries them; a click opens them by workspace — the port, what listens over where, Preview and Stop — with
// the machine's other listeners folded. A stop asks first, naming the process and the port.

import { useState } from "react";
import { actions } from "@/app/store";
import type { Machine } from "@/features/roster/api";
import { useSharing } from "@/features/roster/useSharing";
import { previewServed } from "@/features/triggers/offer";
import { t } from "@/lib/i18n";
import { useQuery } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { ago } from "@/lib/time";
import { Icon, Popover, Tip } from "@/ui";
import { fetchPorts, stopPort } from "./api";
import { paneToOpen } from "./load";
import { portGroups, portsTip, processWithPid, processWords, stopAsk, stoppedWords, workspacePorts, type MachinePort } from "./ports";

export default function Ports({ machine, windows, viewOnly }: { machine: Machine; windows: boolean; viewOnly: boolean }) {
  const [open, setOpen] = useState(false);
  const { machineLabel } = useSharing();
  const count = workspacePorts(machine.sessions);
  const reading = useQuery(() => fetchPorts(machine.id), [machine.id], { every: 5000, enabled: open && !windows });
  if (windows) {
    return (
      <Tip text={t("Not on Windows yet: Windows does not say which folder a listening process runs in")} side="top">
        <span className="status-item status-ports is-off" tabIndex={0}>
          <Icon name="plug" size={12} />
          <span className="status-num">—</span>
        </span>
      </Tip>
    );
  }
  const other = reading.data?.read ? portGroups(machine.sessions, reading.data.ports).other.length : null;
  return (
    <Popover
      open={open}
      onOpenChange={setOpen}
      side="top"
      align="end"
      className="ports-pop"
      label={t("Ports on {machine}", { machine: machineLabel(machine) })}
      tip={portsTip(count, other)}
      trigger={
        <button type="button" className={`btn ghost sm status-item status-ports${count ? " has-ports" : ""}`} aria-label={t("Ports on {machine}", { machine: machineLabel(machine) })}>
          <Icon name="plug" size={12} />
          <span className="status-num">{count}</span>
        </button>
      }
    >
      <PortsList machine={machine} ports={reading.data?.ports ?? null} at={reading.data?.at} error={reading.data?.error || reading.error} viewOnly={viewOnly} onClose={() => setOpen(false)} onStopped={reading.reload} />
    </Popover>
  );
}

function PortsList({ machine, ports, at, error, viewOnly, onClose, onStopped }: { machine: Machine; ports: MachinePort[] | null; at?: string; error?: string; viewOnly: boolean; onClose: () => void; onStopped: () => void }) {
  const { machineLabel, sessionHidden, folderName, redact } = useSharing();
  const [showOther, setShowOther] = useState(false);
  const name = machineLabel(machine);
  const { groups, other } = portGroups(machine.sessions, ports ?? []);
  const inWorkspaces = groups.reduce((n, g) => n + g.ports.length, 0);
  const stop = (port: MachinePort) => {
    onClose();
    actions.ask(redact(stopAsk(port, name)), t("Stop it"), () => {
      stopPort(machine.id, port)
        .then(() => {
          actions.notify(stoppedWords(port), { brief: true });
          onStopped();
        })
        .catch((e) => actions.notify(errMsg(e), { error: true }));
    });
  };
  const row = (port: MachinePort) => (
    <div key={`${port.pid}:${port.port}`} className="port-row">
      <span className="port-number">{port.port}</span>
      <span className="port-what">
        <span className="port-process">{processWords(port)}</span>
        <span className="port-address">{port.address}</span>
      </span>
      {!viewOnly && (
        <span className="port-actions">
          <Tip text={t("Open it through this cockpit, in a tab of its own")} side="top">
            <button type="button" className="btn ghost sm" onClick={() => void previewServed(machine.id, port.port)}>
              <Icon name="external-link" size={12} />
              {t("Preview")}
            </button>
          </Tip>
          <Tip text={t("Stop {process}", { process: processWithPid(port) })} side="top">
            <button type="button" className="btn ghost sm port-stop" onClick={() => stop(port)}>
              <Icon name="square" size={12} />
              {t("Stop")}
            </button>
          </Tip>
        </span>
      )}
    </div>
  );
  return (
    <div className="ports-list">
      <div className="load-head">
        <Icon name="plug" size={14} />
        <strong>{t("Ports")}</strong>
        <span className="load-head-sum">{ports ? portsTip(inWorkspaces, other.length) : t("Looking at the ports…")}</span>
      </div>
      {error && <div className="load-empty is-error">{redact(error)}</div>}
      {ports && groups.length === 0 && <div className="load-empty">{t("No ports listening in your workspaces.")}</div>}
      {groups.map(({ session, ports: listed }) => {
        const hidden = sessionHidden(machine.id, session);
        const pane = paneToOpen(session);
        return (
          <div key={session.id} className="port-group">
            <button
              type="button"
              className="btn ghost sm port-group-head"
              disabled={!pane}
              onClick={() => {
                if (!pane) return;
                onClose();
                actions.openPane(machine.id, pane);
              }}
            >
              <Icon name="folder" size={12} />
              <span className="port-group-name">{hidden ? folderName(machine.id, session.folder || session.dir || "", session.name) : session.name}</span>
              <span className="badge quiet">{listed.length}</span>
            </button>
            {listed.map(row)}
          </div>
        );
      })}
      {other.length > 0 && (
        <div className="port-group is-other">
          <button type="button" className="btn ghost sm port-group-head" aria-expanded={showOther} onClick={() => setShowOther(!showOther)}>
            <Icon name={showOther ? "chevron-down" : "chevron-right"} size={12} />
            <span className="port-group-name">{t("Other processes")}</span>
            <span className="badge quiet">{other.length}</span>
          </button>
          {showOther && other.map(row)}
        </div>
      )}
      {at && <div className="load-foot">{t("Read {ago} on {machine}", { ago: ago(at), machine: name })}</div>}
    </div>
  );
}
