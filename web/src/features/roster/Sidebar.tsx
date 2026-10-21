// The machines sidebar, radar-style: its head with the order, the settings, the schedules, the usage and
// "+ PC"; then every machine with its vitals, its menu and its rows, the busiest first.

import { useCallback, useEffect, useState } from "react";
import { act } from "@/app/act";
import { useCurrent } from "@/app/current";
import { actions } from "@/app/store";
import { usePref } from "@/lib/prefs";
import { Icon, Notice, useDismiss } from "@/ui";
import { removeMachine, type Machine, type Session } from "./api";
import { HealthButton } from "./HealthButton";
import { MachineMenu } from "./MachineMenu";
import { DEFAULT_RADAR, summary, type RadarPrefs } from "./radar";
import { RadarSettings } from "./RadarSettings";
import { Rows, StateMarks, type Current } from "./rows";
import { moving, useClock, useSpinner } from "./useClocks";
import { useRoster } from "./useRoster";

export type { Current } from "./rows";

const nextOrder = { active: "recent", recent: "herdr", herdr: "active" } as const;
const orderTitle = { active: "Grouped by activity — click for a flat list by activity", recent: "Flat, by activity — click for herdr's own order", herdr: "herdr's own order — click to group by activity" };
const orderIcon = { active: "arrow-up-down", recent: "list", herdr: "layers" } as const;

export default function Sidebar() {
  const { machines, latest } = useRoster();
  const serverVersion = __REMUDA_VERSION__ === "dev" ? undefined : __REMUDA_VERSION__;
  const located = useCurrent();
  const current: Current = located ? { machineId: located.machine.id, sessionId: located.session.id, paneId: located.pane.id } : null;
  const [prefs, onPrefs] = usePref<RadarPrefs>("radar", DEFAULT_RADAR);
  const [collapsed, setCollapsed] = usePref<Record<string, boolean>>("collapsed", {});
  const [wsFolded, setWsFolded] = usePref<Record<string, boolean>>("wsFolded", {});
  const onToggle = (id: string) => setCollapsed((c) => ({ ...c, [id]: !c[id] }));
  const onFold = (key: string) => setWsFolded((f) => ({ ...f, [key]: !f[key] }));
  const onMenu = (m: Machine, s: Session, x: number, y: number) => actions.openMenu({ machine: m, session: s, x, y });
  const onRemoveMachine = (m: Machine) => actions.ask(`Remove machine “${m.label}”? Nothing on the PC is touched.`, "Remove", () => void act(removeMachine(m.id)));
  const step = useSpinner(moving(machines));
  const now = useClock(machines);

  // the popover is fixed at its button (the sidebar's scroll would clip it); a dialog opening over the page sets #root inert, which closes it too
  const [settings, setSettings] = useState<{ x: number; y: number } | null>(null);
  const closeSettings = useCallback(() => setSettings(null), []);
  useDismiss(!!settings, closeSettings, [".radar-settings", ".radar-settings-btn", ".sel-pop"]);
  useEffect(() => {
    const root = document.getElementById("root");
    if (!settings || !root) return;
    const mo = new MutationObserver(() => root.hasAttribute("inert") && setSettings(null));
    mo.observe(root, { attributes: true, attributeFilter: ["inert"] });
    return () => mo.disconnect();
  }, [settings]);

  return (
    <>
      <div className="sidebar-head">
        <span>Machines</span>
        <span className="sidebar-head-actions">
          <button type="button" className={`icon-btn${prefs.order !== "active" ? " on" : ""}`} title={orderTitle[prefs.order]} onClick={() => onPrefs({ ...prefs, order: nextOrder[prefs.order] })}>
            <Icon name={orderIcon[prefs.order]} size={14} />
          </button>
          <button
            type="button"
            className={`icon-btn radar-settings-btn${settings ? " on" : ""}`}
            title="Sidebar settings"
            aria-expanded={!!settings}
            onClick={(e) => {
              const r = e.currentTarget.getBoundingClientRect();
              setSettings(settings ? null : { x: Math.min(r.left, window.innerWidth - 246), y: r.bottom });
            }}
          >
            <Icon name="sliders" size={14} />
          </button>
          {settings && <RadarSettings at={settings} prefs={prefs} onPrefs={onPrefs} />}
          <button type="button" className="icon-btn" onClick={() => actions.openPanel("schedules")} title="Schedules: prompts sent to agents on a timetable">
            <Icon name="calendar" size={14} />
          </button>
          <button type="button" className="icon-btn" onClick={() => actions.openPanel("usage")} title="Usage: what the agents cost, by day, project and session">
            <Icon name="chart" size={14} />
          </button>
          <button type="button" className="sidebar-head-btn" onClick={() => actions.openDialog({ kind: "machine" })} title="Add a remote PC over SSH">
            <Icon name="plus" size={12} /> PC
          </button>
        </span>
      </div>
      {machines.map((m) => {
        const folded = !!collapsed[m.id];
        const n = summary(m);
        return (
          <div key={m.id} className={`machine${folded ? " folded" : ""}`}>
            <div className="machine-head">
              <button type="button" className="machine-toggle" title={folded ? "Show workspaces" : "Hide workspaces"} aria-expanded={!folded} onClick={() => onToggle(m.id)}>
                <span className={`machine-state is-${m.state}`} title={m.error || m.state} />
                <span className="machine-label" title={m.host}>
                  {m.label}
                </span>
                {folded && (
                  <span className="machine-summary">
                    <StateMarks working={n.working} blocked={n.blocked} done={n.done} step={step} />
                    {!n.working && !n.blocked && !n.done && m.sessions.length > 0 && <span className="machine-count">{m.sessions.length}</span>}
                  </span>
                )}
                <span className={`chevron${folded ? "" : " open"}`} aria-hidden="true">
                  <Icon name="chevron-right" size={12} />
                </span>
              </button>
              {m.source === "herdr" && (
                <span className="machine-tag" title="Mirrored from `herdr machine list`">
                  herdr
                </span>
              )}
              <HealthButton machine={m} latest={latest} serverVersion={serverVersion} />
              <button type="button" className="icon-btn" title={`New workspace on ${m.label}`} disabled={m.state !== "connected"} onClick={() => actions.openDialog({ kind: "workspace", machineId: m.id })}>
                <Icon name="plus" size={14} />
              </button>
              <MachineMenu machine={m} onRename={() => actions.openDialog({ kind: "renameMachine", machine: m })} onRemove={m.id !== "local" && m.source !== "herdr" ? () => onRemoveMachine(m) : undefined} />
            </div>
            {m.state === "error" && (
              <Notice tone="error" className="machine-error">
                {m.error}
              </Notice>
            )}
            {m.state === "connecting" && (
              <Notice tone="hint" className="machine-error dim">
                connecting…
              </Notice>
            )}
            {!folded && <Rows machine={m} current={current} prefs={prefs} step={step} now={now} onSelect={actions.selectPane} onMenu={onMenu} folded={wsFolded} onFold={onFold} />}
          </div>
        );
      })}
    </>
  );
}
