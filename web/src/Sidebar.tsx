// The machines sidebar, radar-style: one row per agent — a mark that says
// what it is doing, its vendor's logo, its title in the state's colour —
// under workspace headers, worktrees as a tree, the busiest first. A
// finished agent keeps its tick until the pane is looked at; one that asks
// keeps its question until it works again; a parked one cools with time.

import { useCallback, useEffect, useState, type KeyboardEvent } from "react";
import type { Machine, Session } from "./api";
import { fillClass } from "./Usage";
import Logo from "./Logo";
import Select from "./Select";
import {
  agentRows,
  brandColor,
  displayOf,
  groups,
  markOf,
  recentRows,
  rowLabel,
  SPIN_MS,
  summary,
  type AgentRow,
  type Display,
  type Group,
  type RadarPrefs,
} from "./radar";
import { basename, usePref } from "./prefs";
import { ago } from "./time";
import { Notice, StatusDot, useDismiss } from "./ui";
import Icon from "./icons";

export type Current = { machineId: string; sessionId: string; paneId: string } | null;

export default function Sidebar({ machines, current, prefs, collapsed, onToggle, onSelect, onMenu, onNewWorkspace, onRemoveMachine, onAddMachine, onPrefs, onSchedules, onUsage }: {
  machines: Machine[];
  current: Current;
  prefs: RadarPrefs;
  collapsed: Record<string, boolean>;
  onToggle: (machineId: string) => void;
  onSelect: (machineId: string, paneId: string) => void;
  onMenu: (m: Machine, s: Session, x: number, y: number) => void;
  onNewWorkspace: (m: Machine) => void;
  onRemoveMachine: (m: Machine) => void;
  onAddMachine: () => void;
  onPrefs: (next: RadarPrefs) => void;
  onSchedules: () => void;
  onUsage: () => void;
}) {
  // one clock for every spinner and pulse; it only runs while something moves
  const moving = machines.some((m) => m.sessions.some((s) => s.tabs.some((t) => t.panes.some((p) => p.radar?.state === "working" || p.radar?.state === "blocked"))));
  const [step, setStep] = useState(0);
  useEffect(() => {
    if (!moving) return;
    const id = setInterval(() => setStep((s) => (s + 1) % 1_000_000), SPIN_MS);
    return () => clearInterval(id);
  }, [moving]);
  // idle rows cool with the clock, not with events: re-read them now and then
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 30_000);
    return () => clearInterval(id);
  }, []);
  useEffect(() => setNow(Date.now()), [machines]);

  // workspaces folded shut, like the machines above them
  const [wsFolded, setWsFolded] = usePref<Record<string, boolean>>("wsFolded", {});
  const onFold = (key: string) => setWsFolded((f) => ({ ...f, [key]: !f[key] }));

  // the settings popover is fixed at its button: inside the sidebar it would
  // be clipped by the sidebar's own scrolling. A pointer outside closes it —
  // not one on its own button (that toggles) or inside its dropdowns (a
  // portal) — and so does a dialog opening over the page.
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

  const nextOrder = { active: "recent", recent: "herdr", herdr: "active" } as const;
  const orderTitle = { active: "Grouped by activity — click for a flat list by activity", recent: "Flat, by activity — click for herdr's own order", herdr: "herdr's own order — click to group by activity" };
  const orderIcon = { active: "arrow-up-down", recent: "list", herdr: "layers" } as const;

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
          <button type="button" className="icon-btn" onClick={onSchedules} title="Schedules: prompts sent to agents on a timetable">
            <Icon name="calendar" size={14} />
          </button>
          <button type="button" className="icon-btn" onClick={onUsage} title="Usage: what the agents cost, by day, project and session">
            <Icon name="chart" size={14} />
          </button>
          <button type="button" className="sidebar-head-btn" onClick={onAddMachine} title="Add a remote PC over SSH">
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
                    {n.working > 0 && <span className="radar-mark working">{markOf("working", step)} {n.working}</span>}
                    {n.blocked > 0 && <span className="radar-mark blocked">? {n.blocked}</span>}
                    {n.done > 0 && <span className="radar-mark done"><Icon name="check" size={12} /> {n.done}</span>}
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
              <button type="button" className="icon-btn" title={`New workspace on ${m.label}`} disabled={m.state !== "connected"} onClick={() => onNewWorkspace(m)}>
                <Icon name="plus" size={14} />
              </button>
              {m.id !== "local" && m.source !== "herdr" && (
                <button type="button" className="icon-btn danger" title="Remove machine (nothing on the PC is touched)" onClick={() => onRemoveMachine(m)}>
                  <Icon name="x" size={14} />
                </button>
              )}
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
            {!folded && (
              <div className="ws-tree">
                {prefs.order === "recent" ? (
                  <Flat machine={m} current={current} prefs={prefs} step={step} now={now} onSelect={onSelect} onMenu={onMenu} folded={wsFolded} onFold={onFold} />
                ) : (
                  groups(m, prefs).map((g) => (
                    <GroupView key={g.session.id} machine={m} group={g} nested={false} current={current} prefs={prefs} step={step} now={now} onSelect={onSelect} onMenu={onMenu} folded={wsFolded} onFold={onFold} />
                  ))
                )}
              </div>
            )}
          </div>
        );
      })}
    </>
  );
}

type RowProps = {
  machine: Machine;
  current: Current;
  prefs: RadarPrefs;
  step: number;
  now: number;
  onSelect: (machineId: string, paneId: string) => void;
  onMenu: (m: Machine, s: Session, x: number, y: number) => void;
  folded: Record<string, boolean>; // workspaces folded shut, by "machine|workspace"
  onFold: (key: string) => void;
};

const foldKey = (m: Machine, s: Session) => `${m.id}|${s.id}`;

// a row is a button made of a div: Enter or Space on the row itself picks
// it; the same keys on a button inside it are that button's
function rowKey(e: KeyboardEvent<HTMLDivElement>, pick: () => void) {
  if (e.target !== e.currentTarget || (e.key !== "Enter" && e.key !== " ")) return;
  e.preventDefault();
  pick();
}

// what a folded group still says: its agents' states, and its worktrees
function groupSummary(g: Group): { working: number; blocked: number; done: number; agents: number; children: number } {
  const n = { working: 0, blocked: 0, done: 0, agents: 0, children: 0 };
  const walk = (x: Group) => {
    for (const r of x.rows) {
      if (!r.pane.agent) continue;
      n.agents++;
      const st = r.pane.radar?.state;
      if (st === "working" || st === "blocked" || st === "done") n[st]++;
    }
    for (const c of x.children) {
      n.children++;
      walk(c);
    }
  };
  walk(g);
  return n;
}

function GroupView({ machine: m, group: g, nested, ...rest }: RowProps & { group: Group; nested: boolean }) {
  const foldable = g.rows.length > 0 || g.children.length > 0;
  const isFolded = foldable && !!rest.folded[foldKey(m, g.session)];
  return (
    <div className={`ws-group${isFolded ? " folded" : ""}`}>
      <WorkspaceHead machine={m} session={g.session} nested={nested} rows={g.rows} summary={isFolded ? groupSummary(g) : null} foldable={foldable} {...rest} />
      {!isFolded && g.rows.length > 0 && (
        <div className={`radar-rows${nested ? " nested" : ""}`}>
          {g.rows.map((r) => (
            <AgentRowView key={r.pane.id} machine={m} row={r} workspace={g.session.name} {...rest} />
          ))}
        </div>
      )}
      {!isFolded && g.children.length > 0 && (
        <div className="ws-children">
          {g.children.map((c) => (
            <GroupView key={c.session.id} machine={m} group={c} nested {...rest} />
          ))}
        </div>
      )}
    </div>
  );
}

function Flat({ machine: m, ...rest }: RowProps) {
  const rows = recentRows(m, rest.prefs);
  const quiet = m.sessions.filter((s) => agentRows(s, rest.prefs).length === 0);
  return (
    <>
      {rows.map((r) => (
        <AgentRowView key={`p:${r.pane.id}`} machine={m} row={r} workspace="" showWorkspace {...rest} />
      ))}
      {quiet.map((s) => (
        <WorkspaceHead key={`s:${s.id}`} machine={m} session={s} nested={false} rows={[]} {...rest} />
      ))}
    </>
  );
}

// A workspace's line: its name, dim, with herdr's own status for one that
// runs no agent (a shell is still a place to go), and the ⋯ menu.
function WorkspaceHead({ machine: m, session: s, nested, rows, summary, foldable, current, step, onSelect, onMenu, folded, onFold }: RowProps & {
  session: Session;
  nested: boolean;
  rows: AgentRow[];
  summary?: ReturnType<typeof groupSummary> | null; // set while folded
  foldable?: boolean;
}) {
  const rep = representative(s);
  // a folded group's selected agent has no row of its own: the head shows it
  const inside = summary ? current?.machineId === m.id && current.sessionId === s.id : false;
  const selected = inside || (current?.machineId === m.id && current.sessionId === s.id && !rows.some((r) => r.pane.id === current.paneId));
  const folder = s.worktree?.checkout_path ?? rep?.cwd ?? "";
  const isFolded = !!summary;
  return (
    <div
      className={`ws-row radar-head${selected ? " sel" : ""}${nested ? " nested" : ""}${rows.length === 0 ? " quiet" : ""}${isFolded ? " is-folded" : ""}`}
      role="button"
      tabIndex={0}
      onClick={() => rep && onSelect(m.id, rep.id)}
      onKeyDown={(e) => rowKey(e, () => rep && onSelect(m.id, rep.id))}
      title={folder}
    >
      {foldable ? (
        <button
          type="button"
          className={`icon-btn ws-fold${isFolded ? "" : " open"}`}
          title={isFolded ? "Show agents" : "Hide agents"}
          aria-expanded={!isFolded}
          onClick={(e) => {
            e.stopPropagation();
            onFold(foldKey(m, s));
          }}
        >
          <Icon name="chevron-right" size={12} />
        </button>
      ) : (
        rows.length === 0 && <StatusDot status={s.status} />
      )}
      <span className="ws-copy">
        <span className="ws-label">
          {s.name}
          {s.review?.fresh ? (
            <span className="ws-badge" title={`${s.review.fresh} file${s.review.fresh === 1 ? "" : "s"} changed since you last reviewed`}>
              {s.review.fresh}
            </span>
          ) : null}
        </span>
        {rows.length === 0 && (
          <span className="ws-sub">
            {s.tabs.length > 1 && <span>{s.tabs.length} tabs</span>}
            <span className="ws-folder">{basename(folder)}</span>
          </span>
        )}
      </span>
      {summary && (
        <span className="machine-summary ws-summary">
          {summary.working > 0 && <span className="radar-mark working">{markOf("working", step)} {summary.working}</span>}
          {summary.blocked > 0 && <span className="radar-mark blocked">? {summary.blocked}</span>}
          {summary.done > 0 && <span className="radar-mark done"><Icon name="check" size={12} /> {summary.done}</span>}
          {!summary.working && !summary.blocked && !summary.done && (
            <span className="machine-count">
              {summary.agents}
              {summary.children ? ` · ${summary.children} wt` : ""}
            </span>
          )}
        </span>
      )}
      <button
        type="button"
        className="icon-btn ws-menu-btn"
        title="Workspace actions"
        aria-haspopup="menu"
        onClick={(e) => {
          e.stopPropagation();
          const r = e.currentTarget.getBoundingClientRect();
          onMenu(m, s, r.right, r.bottom);
        }}
      >
        <Icon name="more" size={14} />
      </button>
    </div>
  );
}

function AgentRowView({ machine: m, row: r, workspace, showWorkspace, current, prefs, step, now, onSelect, onMenu }: RowProps & { row: AgentRow; workspace: string; showWorkspace?: boolean }) {
  const p = r.pane;
  const d: Display = p.agent ? displayOf(p, prefs, now) : "idle";
  const selected = current?.machineId === m.id && current.paneId === p.id;
  const label = rowLabel(p, r.tab, workspace, prefs);
  const hue = p.agent ? brandColor(p.agent) : undefined;
  const alert = p.radar?.alert;
  const style = d === "working" && hue && !alert ? { color: hue } : undefined;
  return (
    <div
      className={`radar-row is-${d}${selected ? " sel" : ""}${p.agent ? "" : " shell"}${alert ? " is-alert" : ""}`}
      role="button"
      tabIndex={0}
      onClick={() => onSelect(m.id, p.id)}
      onKeyDown={(e) => rowKey(e, () => onSelect(m.id, p.id))}
      title={`${p.agent || "shell"} · ${alert ? alert : stateWords(d, p.radar?.last_active, now)}\n${p.cwd}`}
    >
      <span className="radar-mark" style={style}>
        {alert ? "!" : p.agent ? markOf(d, step) : "$"}
      </span>
      {p.agent && <Logo agent={p.agent} />}
      <span className="radar-title" style={style}>
        {label}
      </span>
      {p.usage && p.usage.window > 0 && p.usage.context / p.usage.window >= 0.5 && (
        <span className={`radar-ctx${fillClass(p.usage.context / p.usage.window)}`} title={`Context ${Math.round((100 * p.usage.context) / p.usage.window)}% full · ${p.usage.name} · $${p.usage.cost.toFixed(2)}`}>
          {Math.round((100 * p.usage.context) / p.usage.window)}%
        </span>
      )}
      {showWorkspace && (
        <>
          <span className="radar-ws">{r.session.name}</span>
          <button
            type="button"
            className="icon-btn ws-menu-btn"
            title="Workspace actions"
            aria-haspopup="menu"
            onClick={(e) => {
              e.stopPropagation();
              const b = e.currentTarget.getBoundingClientRect();
              onMenu(m, r.session, b.right, b.bottom);
            }}
          >
            <Icon name="more" size={14} />
          </button>
        </>
      )}
    </div>
  );
}

function RadarSettings({ at, prefs, onPrefs }: { at: { x: number; y: number }; prefs: RadarPrefs; onPrefs: (p: RadarPrefs) => void }) {
  return (
    <div className="row-menu radar-settings" role="dialog" aria-label="Sidebar settings" style={{ left: at.x, top: at.y }}>
      <label>
        Order
        <Select
          value={prefs.order}
          onChange={(order) => onPrefs({ ...prefs, order })}
          options={[
            { value: "active", label: "grouped by activity", icon: <Icon name="arrow-up-down" size={14} /> },
            { value: "recent", label: "flat, by activity", icon: <Icon name="list" size={14} /> },
            { value: "herdr", label: "herdr's own order", icon: <Icon name="layers" size={14} /> },
          ]}
        />
      </label>
      <label>
        Row label
        <Select
          value={prefs.rowLabel}
          onChange={(rowLabel) => onPrefs({ ...prefs, rowLabel })}
          options={[
            { value: "title", label: "the session's title" },
            { value: "tab", label: "the tab's name" },
            { value: "both", label: "tab · title" },
          ]}
        />
      </label>
      <label>
        Fresh for (min)
        <MinutesInput value={prefs.freshMinutes} onChange={(freshMinutes) => onPrefs({ ...prefs, freshMinutes })} />
      </label>
      <label>
        Stale after (min)
        <MinutesInput value={prefs.staleMinutes} onChange={(staleMinutes) => onPrefs({ ...prefs, staleMinutes })} />
      </label>
      <label className="row">
        <input type="checkbox" checked={prefs.shells} onChange={(e) => onPrefs({ ...prefs, shells: e.target.checked })} /> list shell panes too
      </label>
    </div>
  );
}

// a number field that lets you clear it and retype: the text is local, a
// whole positive number is saved as it is typed, anything else is dropped
// on the way out
function MinutesInput({ value, onChange }: { value: number; onChange: (n: number) => void }) {
  const [text, setText] = useState(String(value));
  useEffect(() => setText(String(value)), [value]);
  return (
    <input
      type="number"
      min={1}
      inputMode="numeric"
      value={text}
      onChange={(e) => {
        setText(e.target.value);
        const n = Number(e.target.value);
        if (e.target.value !== "" && Number.isInteger(n) && n > 0) onChange(n);
      }}
      onBlur={() => setText(String(value))}
    />
  );
}

function representative(s: Session) {
  const tab = s.tabs.find((t) => t.id === s.active_tab_id) ?? s.tabs[0];
  if (!tab) return null;
  return tab.panes.find((p) => p.focused) ?? tab.panes.find((p) => p.agent) ?? tab.panes[0] ?? null;
}

function stateWords(d: Display, last: number | undefined, now: number): string {
  const when = last ? ` · last turn ${ago(last, now)}` : "";
  switch (d) {
    case "working":
      return "working";
    case "blocked":
      return "waiting on you";
    case "done":
      return `done${when}`;
    case "idle_fresh":
      return `idle, just stopped${when}`;
    case "idle_stale":
      return `idle for a long time${when}`;
    case "unknown":
      return "state unknown";
    default:
      return `idle${when}`;
  }
}

// the colour of a context fill: calm below 70%, amber, then red near the edge
