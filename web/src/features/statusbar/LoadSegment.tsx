// The machine's load in the status bar: "CPU 12 % · 3.8 GB", what the pane's machine's herdr panes weigh on it, read
// every 5 s while the page has the focus and every 30 s while it only shows; a click opens its workspaces by memory,
// each with its heaviest process and its terminals, a click on one opening it.

import { useState } from "react";
import { actions } from "@/app/store";
import type { Machine } from "@/features/roster/api";
import { useSharing } from "@/features/roster/useSharing";
import { t } from "@/lib/i18n";
import { useQuery } from "@/lib/query";
import { Icon, Popover, Tip } from "@/ui";
import { fetchLoad } from "./api";
import { cpuBarWords, cpuLevel, cpuWords, footWords, graver, loadTip, memLevel, memWords, paneToOpen, terminalName, workspaceLine, type Usage, type WorkspaceLoad } from "./load";
import { useFocused } from "./useFocused";

export default function Load({ machine, windows }: { machine: Machine; windows: boolean }) {
  const focused = useFocused();
  const [open, setOpen] = useState(false);
  const { machineLabel } = useSharing();
  const name = machineLabel(machine);
  const away = machine.state !== "connected";
  const query = useQuery(() => fetchLoad(machine.id), [machine.id], { every: focused || open ? 5000 : 30000, enabled: !windows && !away });
  const usage = query.data;
  if (windows || away) {
    return (
      <Tip text={windows ? t("Not on Windows yet: Git Bash's ps says nothing of a process's CPU or memory") : t("{machine} is away: its load is read once it answers again", { machine: name })} side="top">
        <span className="status-item status-load is-off" tabIndex={0}>
          <Icon name="activity" size={12} />
          <span className="status-num">{windows ? t("not on Windows yet") : t("CPU {cpu}", { cpu: "—" })}</span>
        </span>
      </Tip>
    );
  }
  const cpu = usage?.cpu_known ? cpuLevel(usage.total.cpu) : "";
  const mem = usage ? memLevel(usage.total.rss_kb, usage.mem_kb) : "";
  const tone = graver(cpu, mem);
  return (
    <Popover
      open={open && !!usage}
      onOpenChange={setOpen}
      side="top"
      align="end"
      className="load-pop"
      label={t("What herdr's panes weigh on {machine}", { machine: name })}
      tip={usage ? loadTip(usage, name) : query.error || t("Measuring…")}
      trigger={
        <button type="button" className={`btn ghost sm status-item status-load${tone ? ` is-${tone}` : ""}`} aria-label={t("The load on {machine}", { machine: name })}>
          <Icon name="activity" size={12} />
          <span className="status-num">
            <span className={cpu ? `is-${cpu}` : undefined}>{usage ? cpuBarWords(usage) : t("CPU {cpu}", { cpu: "…" })}</span>
            <span className="status-sep">·</span>
            <span className={mem ? `is-${mem}` : undefined}>{usage ? memWords(usage.total.rss_kb) : "…"}</span>
          </span>
        </button>
      }
    >
      {usage && <LoadList usage={usage} machine={machine} onPick={() => setOpen(false)} />}
    </Popover>
  );
}

// LoadList is the popup: the workspaces by memory, herdr's own at the foot, what the CPU share means
function LoadList({ usage, machine, onPick }: { usage: Usage; machine: Machine; onPick: () => void }) {
  const { sessionHidden, folderName } = useSharing();
  const [unfolded, setUnfolded] = useState<string | null>(null);
  const byId = new Map(machine.sessions.map((s) => [s.id, s]));
  const nameOf = (ws: WorkspaceLoad) => {
    const session = byId.get(ws.id);
    if (!session) return t("remuda's own");
    return sessionHidden(machine.id, session) ? folderName(machine.id, session.folder || session.dir || "", session.name) : session.name;
  };
  const open = (paneId: string | undefined) => {
    if (!paneId) return;
    onPick();
    actions.openPane(machine.id, paneId);
  };
  return (
    <div className="load-list">
      <div className="load-head">
        <Icon name="activity" size={14} />
        <strong>{t("Load")}</strong>
        <span className="load-head-sum">{t("CPU {cpu} · {mem} of {total}", { cpu: usage.cpu_known ? cpuWords(usage.total.cpu) : "…", mem: memWords(usage.total.rss_kb), total: memWords(usage.mem_kb) })}</span>
      </div>
      {usage.workspaces.length === 0 && <div className="load-empty">{t("No pane of herdr's runs anything on this machine now.")}</div>}
      {usage.workspaces.map((ws) => {
        const session = byId.get(ws.id);
        const folded = unfolded !== ws.id;
        return (
          <div key={ws.id} className="load-ws">
            <div className="load-row">
              <Tip text={folded ? t("Show its terminals") : t("Hide its terminals")} side="left">
                <button type="button" className="icon-btn sm" aria-expanded={!folded} aria-label={folded ? t("Show its terminals") : t("Hide its terminals")} onClick={() => setUnfolded(folded ? ws.id : null)}>
                  <Icon name={folded ? "chevron-right" : "chevron-down"} size={12} />
                </button>
              </Tip>
              <button type="button" className="btn ghost sm load-ws-name" disabled={!session} onClick={() => open(session && paneToOpen(session))}>
                <span className="load-ws-title">{nameOf(ws)}</span>
                <span className="load-ws-line">{workspaceLine(ws)}</span>
              </button>
              <Numbers cpu={ws.cpu} kb={ws.rss_kb} known={usage.cpu_known} total={usage.mem_kb} />
            </div>
            {!folded &&
              ws.terminals.map((term) => (
                <div key={term.pane} className="load-row is-terminal">
                  <button type="button" className="btn ghost sm load-ws-name" onClick={() => open(term.pane)}>
                    <span className="load-ws-line">{terminalName(session, term)}</span>
                  </button>
                  <Numbers cpu={term.cpu} kb={term.rss_kb} known={usage.cpu_known} total={usage.mem_kb} />
                </div>
              ))}
          </div>
        );
      })}
      <div className="load-row is-herdr">
        <span className="load-ws-name">
          <span className="load-ws-title">herdr</span>
          <span className="load-ws-line">{t("the server itself and its attaches")}</span>
        </span>
        <Numbers cpu={usage.herdr.cpu} kb={usage.herdr.rss_kb} known={usage.cpu_known} total={usage.mem_kb} />
      </div>
      <div className="load-foot">{footWords(usage)}</div>
    </div>
  );
}

function Numbers({ cpu, kb, known, total }: { cpu: number; kb: number; known: boolean; total: number }) {
  const cpuTone = known ? cpuLevel(cpu) : "";
  const memTone = memLevel(kb, total);
  return (
    <span className="load-nums">
      <span className={cpuTone ? `is-${cpuTone}` : undefined}>{known ? cpuWords(cpu) : "…"}</span>
      <span className={memTone ? `is-${memTone}` : undefined}>{memWords(kb)}</span>
    </span>
  );
}
