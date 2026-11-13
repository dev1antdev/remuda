// The machines sidebar, radar-style: its head with a + menu (a workspace, a recipe, a scene, a PC) and a ⋯ menu (message
// several agents, fold or open every folder, the order, the settings); then "Needs you" and the pinned agents; then
// every machine with its vitals, its menu and its folders; and at its foot how many agents run and how many need you.

import { useEffect } from "react";
import { act } from "@/app/act";
import { useCurrent } from "@/app/current";
import { actions, app } from "@/app/store";
import { useMedia } from "@/lib/media";
import { usePref } from "@/lib/prefs";
import { Icon, Menu, Notice, Tip, type MenuItem } from "@/ui";
import { removeMachine, type Machine } from "./api";
import { HealthButton } from "./HealthButton";
import { MachineMenu } from "./MachineMenu";
import { DEFAULT_RADAR, type RadarOrder, type RadarPrefs } from "./radar";
import { Rows, type Current } from "./rows";
import { entries, foldKey, type Folder } from "./workspaces";
import { useSharing } from "./useSharing";
import { hueStyle } from "./hue";
import { moving, useClock, useSpinner } from "./useClocks";
import { NO_MACHINES } from "./server";
import { useRoster } from "./useRoster";
import { useScenes } from "@/features/scenes/api";
import { agentKey, fleetTally, livePins, pinnedRows, PINS_KEY, tallyOf, togglePin, waitingAgents } from "./sidebarAgents";
import { HeadSummary } from "./marks";
import { NeedsYou, PinnedAgents } from "./SidebarGroups";
import { SidebarFoot } from "./SidebarFoot";
import { useQuestions } from "./useQuestions";
import { t } from "@/lib/i18n";

export type { Current } from "./rows";

// ORDERS are the sidebar's orders as its ⋯ menu offers them
const ORDERS: { order: RadarOrder; label: () => string }[] = [
  { order: "active", label: () => t("Grouped by activity") },
  { order: "recent", label: () => t("Flat, by activity") },
  { order: "herdr", label: () => t("herdr's own order") },
  { order: "priority", label: () => t("By priority") },
  { order: "prefix", label: () => t("By ticket prefix") },
];

export default function Sidebar() {
  const { machines, latest, viewOnly, server } = useRoster();
  const scenes = useScenes(!viewOnly);
  const serverVersion = __REMUDA_VERSION__ === "dev" ? undefined : __REMUDA_VERSION__;
  const located = useCurrent();
  const current: Current = located ? { machineId: located.machine.id, sessionId: located.session.id, paneId: located.pane.id } : null;
  const [prefs, onPrefs] = usePref<RadarPrefs>("radar", DEFAULT_RADAR);
  const [collapsed, setCollapsed] = usePref<Record<string, boolean>>("collapsed", {});
  const [wsFolded, setWsFolded] = usePref<Record<string, boolean>>("wsFolded", {});
  const [pins, setPins] = usePref<string[]>(PINS_KEY, []);
  const [subFolded, setSubFolded] = usePref<Record<string, boolean>>("sidebar.subagents.folded", {});
  const onToggle = (id: string) => setCollapsed((c) => ({ ...c, [id]: !c[id] }));
  const onFold = (key: string) => setWsFolded((f) => ({ ...f, [key]: !f[key] }));
  const onFoldSubs = (key: string) => setSubFolded((f) => ({ ...f, [key]: !f[key] }));
  const onPin = viewOnly ? undefined : (key: string) => setPins((list) => togglePin(list, key));
  const onIcon = (m: Machine, f: Folder) => actions.openDialog({ kind: "folder-icon", machineId: m.id, folder: f.path, name: f.name, icon: f.icon });
  // every folder and section of every machine shut, or all open again
  const foldAll = (shut: boolean) =>
    setWsFolded((f) => {
      const next = { ...f };
      for (const m of machines)
        for (const e of entries(m, prefs)) {
          const key = foldKey(m.id, e);
          if (key) next[key] = shut;
        }
      return next;
    });
  const share = useSharing();
  const onRemoveMachine = (m: Machine) => actions.ask(t("Remove machine “{machine}”? Nothing on the PC is touched.", { machine: share.machineLabel(m) }), t("Remove"), () => void act(removeMachine(m.id)));
  // the state dot's tooltip: the reason a machine cannot be reached, its own names masked while sharing the screen
  const stateTitle = (m: Machine) => (m.state === "error" ? share.redact(m.error || t("unreachable")) : m.state === "connecting" ? t("connecting…") : t("connected"));
  const subagentsWork = machines.some((m) => m.sessions.some((s) => s.tabs.some((tab) => tab.panes.some((p) => (p.subagents?.working ?? 0) > 0))));
  const step = useSpinner(moving(machines) || subagentsWork);
  const now = useClock(machines);

  // the asks are read off the screens only while the sidebar is on screen: beside the pane at a desk, the drawer open on a phone
  const narrow = useMedia("(max-width: 920px)");
  const drawer = app.use((s) => s.drawer);
  const full = app.use((s) => s.full);
  const onScreen = !full && (!narrow || drawer);
  const waiting = waitingAgents(machines);
  const askers = onScreen ? waiting.filter((w) => w.pane.radar?.state === "blocked" && !share.sessionHidden(w.machine.id, w.session)).map((w) => ({ key: w.key, machine: w.machine.id, pane: w.pane.id })) : [];
  const questions = useQuestions(askers);
  const selected = current ? agentKey(current.machineId, current.paneId) : null;

  // a pin of an agent gone is let go, so a later pane given the same id does not inherit it
  const kept = livePins(machines, pins);
  useEffect(() => {
    if (kept.length !== pins.length) setPins(kept);
  }, [kept.length, pins.length]); // eslint-disable-line react-hooks/exhaustive-deps

  const sidebarMenu: MenuItem[] = [
    { label: t("Message several agents…"), icon: "send", hint: t("one text to the ones you tick, each in turn"), onSelect: () => actions.openDialog({ kind: "broadcast" }) },
    { sep: true },
    { label: t("Fold every folder"), icon: "chevron-right", hint: t("only the folders and how their agents stand"), onSelect: () => foldAll(true) },
    { label: t("Open every folder"), icon: "chevron-down", onSelect: () => foldAll(false) },
    { sep: true },
    { heading: t("Order") },
    ...ORDERS.map(({ order, label }) => ({ label: label(), checked: prefs.order === order, onSelect: () => onPrefs({ ...prefs, order }) })),
    { sep: true },
    { label: t("Sidebar settings…"), icon: "sliders", hint: t("labels, idle tiers, shells"), onSelect: () => actions.openDialog({ kind: "settings" }) },
  ];

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
            items={sidebarMenu}
          />
        </span>}
      </div>
      <NeedsYou waiting={waiting} questions={questions} share={share} now={now} selected={selected} />
      <PinnedAgents pinned={pinnedRows(machines, kept)} share={share} prefs={prefs} step={step} now={now} selected={selected} onSelect={actions.selectPane} pins={kept} onPin={onPin} subFolded={subFolded} onFoldSubs={onFoldSubs} />
      {server && machines.length === 0 && <p className="ws-empty">{t(NO_MACHINES)}</p>}
      {machines.map((m) => {
        const folded = !!collapsed[m.id];
        const workspaces = m.sessions.filter((s) => !s.scratch).length; // the floating terminal's shell is the drawer's
        const tally = tallyOf(m.sessions);
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
                    <HeadSummary tally={tally} step={step} />
                    {!tally.working && !tally.needs && !tally.done && workspaces > 0 && <span className="machine-count">{workspaces}</span>}
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
                pins={kept}
                onPin={onPin}
                subFolded={subFolded}
                onFoldSubs={onFoldSubs}
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
      <SidebarFoot tally={fleetTally(machines)} oldest={waiting[0]} />
    </>
  );
}
