// The machines sidebar, radar-style: its head with a + menu (a workspace, a PC) and a ⋯ menu (message several agents,
// the order, the settings); then every machine with its vitals, its menu and its rows, the busiest first.

import { act } from "@/app/act";
import { useCurrent } from "@/app/current";
import { actions } from "@/app/store";
import { usePref } from "@/lib/prefs";
import { Icon, Menu, Notice, Tip } from "@/ui";
import { removeMachine, type Machine, type Session } from "./api";
import { HealthButton } from "./HealthButton";
import { MachineMenu } from "./MachineMenu";
import { DEFAULT_RADAR, summary, type RadarPrefs } from "./radar";
import { Rows, StateMarks, type Current } from "./rows";
import { hueStyle } from "./hue";
import { moving, useClock, useSpinner } from "./useClocks";
import { useRoster } from "./useRoster";

export type { Current } from "./rows";

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


  return (
    <>
      <div className="sidebar-head">
        <span>Machines</span>
        <span className="sidebar-head-actions">
          <Menu
            label="New"
            align="end"
            tip="New: a workspace on a machine, or a PC"
            trigger={
              <button type="button" className="icon-btn" aria-label="New">
                <Icon name="plus" size={14} />
              </button>
            }
            items={[
              ...machines.filter((m) => m.state === "connected").map((m) => ({ label: `New workspace on ${m.label}…`, icon: "terminal" as const, onSelect: () => actions.openDialog({ kind: "workspace", machineId: m.id }) })),
              { label: "Agent from recipe…", icon: "sparkles", hint: "a saved agent, model, arguments and first prompt", onSelect: () => actions.openDialog({ kind: "start-recipe" }) },
              { sep: true },
              { label: "Add a PC…", icon: "monitor", hint: "a remote machine over SSH", onSelect: () => actions.openDialog({ kind: "machine" }) },
            ]}
          />
          <Menu
            label="Sidebar"
            align="end"
            tip="Message several agents, the order, the sidebar settings"
            trigger={
              <button type="button" className="icon-btn" aria-label="More">
                <Icon name="more" size={14} />
              </button>
            }
            items={[
              { label: "Message several agents…", icon: "send", hint: "one text to the ones you tick, each in turn", onSelect: () => actions.openDialog({ kind: "broadcast" }) },
              { sep: true },
              { heading: "Order" },
              { label: "Grouped by activity", checked: prefs.order === "active", onSelect: () => onPrefs({ ...prefs, order: "active" }) },
              { label: "Flat, by activity", checked: prefs.order === "recent", onSelect: () => onPrefs({ ...prefs, order: "recent" }) },
              { label: "herdr's own order", checked: prefs.order === "herdr", onSelect: () => onPrefs({ ...prefs, order: "herdr" }) },
              { sep: true },
              { label: "Sidebar settings…", icon: "sliders", hint: "labels, idle tiers, shells", onSelect: () => actions.openDialog({ kind: "settings" }) },
            ]}
          />
        </span>
      </div>
      {machines.map((m) => {
        const folded = !!collapsed[m.id];
        const n = summary(m);
        return (
          <div key={m.id} className={`machine${folded ? " folded" : ""}${m.id !== "local" ? " has-hue" : ""}`} style={hueStyle(m.id)}>
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
              <Tip text={`New workspace on ${m.label}`}>
                <button type="button" className="icon-btn" aria-label={`New workspace on ${m.label}`} disabled={m.state !== "connected"} onClick={() => actions.openDialog({ kind: "workspace", machineId: m.id })}>
                  <Icon name="plus" size={14} />
                </button>
              </Tip>
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
