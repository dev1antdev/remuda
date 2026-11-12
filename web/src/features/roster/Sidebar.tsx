// The machines sidebar, radar-style: its head with a + menu (a workspace, a recipe, a scene, a PC) and a ⋯ menu (message several agents,
// fold or open every folder, the order, the settings); then every machine with its vitals, its menu and its folders.

import { act } from "@/app/act";
import { useCurrent } from "@/app/current";
import { actions } from "@/app/store";
import { usePref } from "@/lib/prefs";
import { Icon, Menu, Notice, Tip } from "@/ui";
import { removeMachine, type Machine } from "./api";
import { HealthButton } from "./HealthButton";
import { MachineMenu } from "./MachineMenu";
import { DEFAULT_RADAR, summary, type RadarPrefs } from "./radar";
import { Rows, StateMarks, type Current } from "./rows";
import { entries, type Folder } from "./workspaces";
import { useSharing } from "./useSharing";
import { hueStyle } from "./hue";
import { moving, useClock, useSpinner } from "./useClocks";
import { NO_MACHINES } from "./server";
import { useRoster } from "./useRoster";
import { useScenes } from "@/features/scenes/api";
import { t } from "@/lib/i18n";

export type { Current } from "./rows";

export default function Sidebar() {
  const { machines, latest, viewOnly, server } = useRoster();
  const scenes = useScenes(!viewOnly);
  const serverVersion = __REMUDA_VERSION__ === "dev" ? undefined : __REMUDA_VERSION__;
  const located = useCurrent();
  const current: Current = located ? { machineId: located.machine.id, sessionId: located.session.id, paneId: located.pane.id } : null;
  const [prefs, onPrefs] = usePref<RadarPrefs>("radar", DEFAULT_RADAR);
  const [collapsed, setCollapsed] = usePref<Record<string, boolean>>("collapsed", {});
  const [wsFolded, setWsFolded] = usePref<Record<string, boolean>>("wsFolded", {});
  const onToggle = (id: string) => setCollapsed((c) => ({ ...c, [id]: !c[id] }));
  const onFold = (key: string) => setWsFolded((f) => ({ ...f, [key]: !f[key] }));
  const onIcon = (m: Machine, f: Folder) => actions.openDialog({ kind: "folder-icon", machineId: m.id, folder: f.path, name: f.name, icon: f.icon });
  // every folder of every machine shut, or all open again
  const foldAll = (shut: boolean) =>
    setWsFolded((f) => {
      const next = { ...f };
      for (const m of machines) for (const e of entries(m, prefs)) if (e.kind === "folder") next[`${m.id}|folder:${e.folder.key}`] = shut;
      return next;
    });
  const share = useSharing();
  const onRemoveMachine = (m: Machine) => actions.ask(t("Remove machine “{machine}”? Nothing on the PC is touched.", { machine: share.machineLabel(m) }), t("Remove"), () => void act(removeMachine(m.id)));
  // the state dot's tooltip: the reason a machine cannot be reached, its own names masked while sharing the screen
  const stateTitle = (m: Machine) => (m.state === "error" ? share.redact(m.error || t("unreachable")) : m.state === "connecting" ? t("connecting…") : t("connected"));
  const step = useSpinner(moving(machines));
  const now = useClock(machines);

  return (
    <>
      <div className="sidebar-head">
        <span>{t("Machines")}</span>
        {!viewOnly && <span className="sidebar-head-actions">
          <Menu
            label={t("New")}
            align="end"
            tip={t("New: a workspace on a machine, a scene, or a PC")}
            trigger={
              <button type="button" className="icon-btn" aria-label={t("New")}>
                <Icon name="plus" size={14} />
              </button>
            }
            items={[
              ...machines.filter((m) => m.state === "connected").map((m) => ({ label: t("New workspace on {machine}…", { machine: share.machineLabel(m) }), icon: "terminal" as const, onSelect: () => actions.openDialog({ kind: "workspace", machineId: m.id }) })),
              { label: t("Agent from recipe…"), icon: "sparkles", hint: t("a saved agent, model, arguments and first prompt"), onSelect: () => actions.openDialog({ kind: "start-recipe" }) },
              ...scenes.flatMap((scene) => (scene.id ? [{ label: t("Scene: {name}…", { name: scene.name }), icon: scene.kind === "tidy" ? ("moon" as const) : ("sun" as const), onSelect: () => actions.openDialog({ kind: "scene-run", id: scene.id ?? "" }) }] : [])),
              { label: t("Scenes…"), icon: "sun", hint: t("a morning set-up, an evening tidy"), onSelect: () => actions.openDialog({ kind: "scenes" }) },
              { sep: true },
              { label: t("Add a PC…"), icon: "monitor", hint: t("a remote machine over SSH"), onSelect: () => actions.openDialog({ kind: "machine" }) },
            ]}
          />
          <Menu
            label={t("Sidebar")}
            align="end"
            tip={t("Message several agents, the order, the sidebar settings")}
            trigger={
              <button type="button" className="icon-btn" aria-label={t("More")}>
                <Icon name="more" size={14} />
              </button>
            }
            items={[
              { label: t("Message several agents…"), icon: "send", hint: t("one text to the ones you tick, each in turn"), onSelect: () => actions.openDialog({ kind: "broadcast" }) },
              { sep: true },
              { label: t("Fold every folder"), icon: "chevron-right", hint: t("only the folders and how their agents stand"), onSelect: () => foldAll(true) },
              { label: t("Open every folder"), icon: "chevron-down", onSelect: () => foldAll(false) },
              { sep: true },
              { heading: t("Order") },
              { label: t("Grouped by activity"), checked: prefs.order === "active", onSelect: () => onPrefs({ ...prefs, order: "active" }) },
              { label: t("Flat, by activity"), checked: prefs.order === "recent", onSelect: () => onPrefs({ ...prefs, order: "recent" }) },
              { label: t("herdr's own order"), checked: prefs.order === "herdr", onSelect: () => onPrefs({ ...prefs, order: "herdr" }) },
              { sep: true },
              { label: t("Sidebar settings…"), icon: "sliders", hint: t("labels, idle tiers, shells"), onSelect: () => actions.openDialog({ kind: "settings" }) },
            ]}
          />
        </span>}
      </div>
      {server && machines.length === 0 && <p className="ws-empty">{t(NO_MACHINES)}</p>}
      {machines.map((m) => {
        const folded = !!collapsed[m.id];
        const n = summary(m);
        const workspaces = m.sessions.filter((s) => !s.scratch).length; // the floating terminal's shell is the drawer's
        return (
          <div key={m.id} className={`machine${folded ? " folded" : ""}${m.id !== "local" ? " has-hue" : ""}`} style={hueStyle(m.id)}>
            <div className="machine-head">
              <button type="button" className={`machine-toggle${folded && current?.machineId === m.id ? " on" : ""}`} title={folded ? t("Show workspaces") : t("Hide workspaces")} aria-expanded={!folded} onClick={() => onToggle(m.id)}>
                <span className={`machine-state is-${m.state}`} title={stateTitle(m)} />
                <span className="machine-label" title={share.machineHidden(m.id) ? undefined : m.host}>
                  {share.machineLabel(m)}
                </span>
                {folded && (
                  <span className="machine-summary">
                    <StateMarks working={n.working} blocked={n.blocked} done={n.done} step={step} />
                    {!n.working && !n.blocked && !n.done && workspaces > 0 && <span className="machine-count">{workspaces}</span>}
                  </span>
                )}
                <span className={`chevron${folded ? "" : " open"}`} aria-hidden="true">
                  <Icon name="chevron-right" size={12} />
                </span>
              </button>
              <HealthButton machine={m} latest={latest} serverVersion={serverVersion} viewOnly={viewOnly} />
              {!viewOnly && <Tip text={t("New workspace on {machine}", { machine: share.machineLabel(m) })}>
                <button type="button" className="icon-btn" aria-label={t("New workspace on {machine}", { machine: share.machineLabel(m) })} disabled={m.state !== "connected"} onClick={() => actions.openDialog({ kind: "workspace", machineId: m.id })}>
                  <Icon name="plus" size={14} />
                </button>
              </Tip>}
              {!viewOnly && <MachineMenu machine={m} onRename={() => actions.openDialog({ kind: "renameMachine", machine: m })} onRemove={m.id !== "local" && m.source !== "herdr" ? () => onRemoveMachine(m) : undefined} />}
            </div>
            {m.state === "error" && (
              <Notice tone="error" className="machine-error">
                {share.redact(m.error ?? "")}
              </Notice>
            )}
            {m.state === "connecting" && (
              <Notice tone="hint" className="machine-error dim">
                {t("connecting…")}
              </Notice>
            )}
            {!folded && (
              <Rows
                machine={m}
                current={current}
                prefs={prefs}
                step={step}
                now={now}
                onSelect={actions.selectPane}
                editable={!viewOnly}
                onIcon={viewOnly ? undefined : onIcon}
                share={share}
                folded={wsFolded}
                onFold={onFold}
              />
            )}
            {!folded && m.state === "connected" && workspaces === 0 && (
              <p className="ws-empty">
                {t("No workspaces here yet.")}{" "}
                {!viewOnly && (
                  <button type="button" className="linkish" onClick={() => actions.openDialog({ kind: "workspace", machineId: m.id })}>
                    {t("New workspace…")}
                  </button>
                )}
              </p>
            )}
          </div>
        );
      })}
    </>
  );
}
