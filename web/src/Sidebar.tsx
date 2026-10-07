// The machines sidebar, radar-style: one row per agent — a mark that says
// what it is doing, its vendor's logo, its title in the state's colour —
// under workspace headers, worktrees as a tree, the busiest first. A
// finished agent keeps its tick until the pane is looked at; one that asks
// keeps its question until it works again; a parked one cools with time.

import { useEffect, useState } from "react";
import type { Machine, Session, Status } from "./api";
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
import Icon from "./icons";

export type Current = { machineId: string; sessionId: string; paneId: string } | null;

export default function Sidebar({ machines, current, prefs, collapsed, onToggle, onSelect, onMenu, onNewWorkspace, onRemoveMachine, onAddMachine, onPrefs, onSchedules }: {
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

  // the settings popover is fixed at its button: inside the sidebar it would
  // be clipped by the sidebar's own scrolling. A click outside closes it —
  // a click inside one of its own dropdowns (a portal) does not.
  // workspaces folded shut, like the machines above them
  const [wsFolded, setWsFolded] = usePref<Record<string, boolean>>("wsFolded", {});
  const onFold = (key: string) => setWsFolded((f) => ({ ...f, [key]: !f[key] }));

  const [settings, setSettings] = useState<{ x: number; y: number } | null>(null);
  useEffect(() => {
    if (!settings) return;
    const close = (e: MouseEvent) => {
      const t = e.target as Element;
      if (t.closest(".radar-settings") || t.closest(".sel-pop")) return;
      setSettings(null);
    };
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && setSettings(null);
    document.addEventListener("mousedown", close);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", close);
      document.removeEventListener("keydown", onKey);
    };
  }, [settings]);

  const nextOrder = { active: "recent", recent: "herdr", herdr: "active" } as const;
  const orderTitle = { active: "Grouped by activity — click for a flat list by activity", recent: "Flat, by activity — click for herdr's own order", herdr: "herdr's own order — click to group by activity" };
  const orderIcon = { active: "arrow-up-down", recent: "list", herdr: "layers" } as const;

  return (
    <>
      <div className="sidebar-head">
        <span>Machines</span>
        <span className="sidebar-head-actions">
          <button className={prefs.order !== "active" ? "on" : ""} title={orderTitle[prefs.order]} onClick={() => onPrefs({ ...prefs, order: nextOrder[prefs.order] })}>
            <Icon name={orderIcon[prefs.order]} size={14} />
          </button>
          <button
            className={settings ? "on" : ""}
            title="Sidebar settings"
            onClick={(e) => {
              const r = e.currentTarget.getBoundingClientRect();
              setSettings(settings ? null : { x: Math.min(r.left, window.innerWidth - 246), y: r.bottom });
            }}
          >
            <Icon name="sliders" size={14} />
          </button>
          {settings && <RadarSettings at={settings} prefs={prefs} onPrefs={onPrefs} />}
          <button onClick={onSchedules} title="Schedules: prompts sent to agents on a timetable">
            <Icon name="calendar" size={14} />
          </button>
          <button onClick={onAddMachine} title="Add a remote PC over SSH">
            <Icon name="plus" size={13} /> PC
          </button>
        </span>
      </div>
      {machines.map((m) => {
        const folded = !!collapsed[m.id];
        const n = summary(m);
        return (
          <div key={m.id} className={`machine${folded ? " folded" : ""}`}>
            <div className="machine-head">
              <button className="machine-toggle" title={folded ? "Show workspaces" : "Hide workspaces"} onClick={() => onToggle(m.id)}>
                <span className={`machine-state is-${m.state}`} title={m.error || m.state} />
                <span className="machine-label" title={m.host}>
                  {m.label}
                </span>
                {folded && (
                  <span className="machine-summary">
                    {n.working > 0 && <span className="radar-mark working">{markOf("working", step)} {n.working}</span>}
                    {n.blocked > 0 && <span className="radar-mark blocked">? {n.blocked}</span>}
                    {n.done > 0 && <span className="radar-mark done">✓ {n.done}</span>}
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
              <button className="icon-btn" title={`New workspace on ${m.label}`} disabled={m.state !== "connected"} onClick={() => onNewWorkspace(m)}>
                <Icon name="plus" size={14} />
              </button>
              {m.id !== "local" && m.source !== "herdr" && (
                <button className="icon-btn" title="Remove machine (nothing on the PC is touched)" onClick={() => onRemoveMachine(m)}>
                  <Icon name="x" size={14} />
                </button>
              )}
            </div>
            {m.state === "error" && <div className="machine-error">{m.error}</div>}
            {m.state === "connecting" && <div className="machine-error dim">connecting…</div>}
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
        <AgentRowView key={r.pane.id} machine={m} row={r} workspace="" showWorkspace {...rest} />
      ))}
      {quiet.map((s) => (
        <WorkspaceHead key={s.id} machine={m} session={s} nested={false} rows={[]} {...rest} />
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
      onKeyDown={(e) => (e.key === "Enter" || e.key === " ") && rep && onSelect(m.id, rep.id)}
      title={folder}
    >
      {foldable ? (
        <button
          type="button"
          className={`ws-fold${isFolded ? "" : " open"}`}
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
          {summary.done > 0 && <span className="radar-mark done">✓ {summary.done}</span>}
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
        className="ws-menu-btn"
        title="Workspace actions"
        onClick={(e) => {
          e.stopPropagation();
          const r = e.currentTarget.getBoundingClientRect();
          onMenu(m, s, r.right, r.bottom);
        }}
      >
        <Icon name="more" size={15} />
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
      onKeyDown={(e) => (e.key === "Enter" || e.key === " ") && onSelect(m.id, p.id)}
      title={`${p.agent || "shell"} · ${alert ? alert : stateWords(d, p.radar?.last_active, now)}\n${p.cwd}`}
    >
      <span className="radar-mark" style={style}>
        {alert ? "!" : p.agent ? markOf(d, step) : "$"}
      </span>
      {p.agent && <Logo agent={p.agent} />}
      <span className="radar-title" style={style}>
        {label}
      </span>
      {showWorkspace && (
        <>
          <span className="radar-ws">{r.session.name}</span>
          <button
            type="button"
            className="ws-menu-btn"
            title="Workspace actions"
            onClick={(e) => {
              e.stopPropagation();
              const b = e.currentTarget.getBoundingClientRect();
              onMenu(m, r.session, b.right, b.bottom);
            }}
          >
            <Icon name="more" size={15} />
          </button>
        </>
      )}
    </div>
  );
}

function RadarSettings({ at, prefs, onPrefs }: { at: { x: number; y: number }; prefs: RadarPrefs; onPrefs: (p: RadarPrefs) => void }) {
  const num = (v: string, fallback: number) => {
    const n = Number(v);
    return Number.isFinite(n) && n > 0 ? n : fallback;
  };
  return (
    <div className="row-menu radar-settings" style={{ left: at.x, top: at.y }}>
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
        <input type="number" min={1} value={prefs.freshMinutes} onChange={(e) => onPrefs({ ...prefs, freshMinutes: num(e.target.value, prefs.freshMinutes) })} />
      </label>
      <label>
        Stale after (min)
        <input type="number" min={1} value={prefs.staleMinutes} onChange={(e) => onPrefs({ ...prefs, staleMinutes: num(e.target.value, prefs.staleMinutes) })} />
      </label>
      <label className="row">
        <input type="checkbox" checked={prefs.shells} onChange={(e) => onPrefs({ ...prefs, shells: e.target.checked })} /> list shell panes too
      </label>
    </div>
  );
}

function StatusDot({ status }: { status: Status }) {
  return <span className={`dot dot-${status}`} />;
}

function representative(s: Session) {
  const tab = s.tabs.find((t) => t.id === s.active_tab_id) ?? s.tabs[0];
  if (!tab) return null;
  return tab.panes.find((p) => p.focused) ?? tab.panes.find((p) => p.agent) ?? tab.panes[0] ?? null;
}

function stateWords(d: Display, last: number | undefined, now: number): string {
  const ago = last ? ` · last turn ${since(now - last)}` : "";
  switch (d) {
    case "working":
      return "working";
    case "blocked":
      return "waiting on you";
    case "done":
      return `done${ago}`;
    case "idle_fresh":
      return `idle, just stopped${ago}`;
    case "idle_stale":
      return `idle for a long time${ago}`;
    case "unknown":
      return "state unknown";
    default:
      return `idle${ago}`;
  }
}

function since(ms: number): string {
  const m = Math.round(ms / 60000);
  if (m < 1) return "just now";
  if (m < 60) return `${m} min ago`;
  const h = Math.round(m / 60);
  if (h < 48) return `${h} h ago`;
  return `${Math.round(h / 24)} days ago`;
}
