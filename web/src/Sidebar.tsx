// The machines sidebar, radar-style: one row per agent — a mark for what it
// is doing, its vendor's logo, its title in the state's colour — under
// workspace headers, worktrees as a tree, the busiest first.

import { useCallback, useEffect, useState, type KeyboardEvent } from "react";
import { refreshHealth, type Health, type Latest, type Machine, type Session } from "./api";
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
import { basename, errMsg, usePref } from "./prefs";
import { ago } from "./time";
import { Notice, StatusDot, useDismiss } from "./ui";
import UpdatesDialog from "./Updates";
import Icon from "./icons";

export type Current = { machineId: string; sessionId: string; paneId: string } | null;

export default function Sidebar({ machines, latest, serverVersion, current, prefs, collapsed, onToggle, onSelect, onMenu, onNewWorkspace, onRemoveMachine, onRenameMachine, onAddMachine, onPrefs, onSchedules, onUsage }: {
  machines: Machine[];
  latest?: Latest;
  serverVersion?: string;
  current: Current;
  prefs: RadarPrefs;
  collapsed: Record<string, boolean>;
  onToggle: (machineId: string) => void;
  onSelect: (machineId: string, paneId: string) => void;
  onMenu: (m: Machine, s: Session, x: number, y: number) => void;
  onNewWorkspace: (m: Machine) => void;
  onRemoveMachine: (m: Machine) => void;
  onRenameMachine: (m: Machine) => void;
  onAddMachine: () => void;
  onPrefs: (next: RadarPrefs) => void;
  onSchedules: () => void;
  onUsage: () => void;
}) {
  const moving = machines.some((m) => m.sessions.some((s) => s.tabs.some((t) => t.panes.some((p) => p.radar?.state === "working" || p.radar?.state === "blocked"))));
  const [step, setStep] = useState(0);
  useEffect(() => {
    if (!moving) return;
    const id = setInterval(() => setStep((s) => (s + 1) % 1_000_000), SPIN_MS);
    return () => clearInterval(id);
  }, [moving]);
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    const id = setInterval(() => setNow(Date.now()), 30_000);
    return () => clearInterval(id);
  }, []);
  useEffect(() => setNow(Date.now()), [machines]);

  const [wsFolded, setWsFolded] = usePref<Record<string, boolean>>("wsFolded", {});
  const onFold = (key: string) => setWsFolded((f) => ({ ...f, [key]: !f[key] }));

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
              <HealthButton machine={m} latest={latest} serverVersion={serverVersion} />
              <button type="button" className="icon-btn" title={`New workspace on ${m.label}`} disabled={m.state !== "connected"} onClick={() => onNewWorkspace(m)}>
                <Icon name="plus" size={14} />
              </button>
              <MachineMenu machine={m} onRename={() => onRenameMachine(m)} onRemove={m.id !== "local" && m.source !== "herdr" ? () => onRemoveMachine(m) : undefined} />
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

// rowKey makes a div row a button: Enter or Space on the row itself picks it, on a button inside it they are that button's
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

// WorkspaceHead is a workspace's line: its name, herdr's own status when it runs no agent, and the ⋯ menu
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

// MinutesInput keeps its text local so the field can be cleared and retyped; a whole positive number saves as typed
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

// ---- a machine's vitals ----
// Claude Code, herdr and remuda versions against the newest known, disk and load: an amber chip when something wants a look

type Concern = { key: string; text: string; bad?: boolean; update?: string };

function concerns(h: Health | undefined, latest: Latest | undefined, serverVersion: string | undefined, remote: boolean): Concern[] {
  if (!h) return [];
  const out: Concern[] = [];
  if (h.claude && latest?.claude && newer(h.claude, latest.claude)) out.push({ key: "claude", text: `Claude Code ${latest.claude} is out (${h.claude} here)`, update: "claude" });
  if (h.codex && latest?.codex && newer(h.codex, latest.codex)) out.push({ key: "codex", text: `Codex ${latest.codex} is out (${h.codex} here)`, update: "codex" });
  if (h.gemini && latest?.gemini && newer(h.gemini, latest.gemini)) out.push({ key: "gemini", text: `Gemini CLI ${latest.gemini} is out (${h.gemini} here)`, update: "gemini" });
  if (h.herdr && latest?.herdr && newer(h.herdr, latest.herdr)) out.push({ key: "herdr", text: `herdr ${latest.herdr} is out (${h.herdr} here)`, update: "herdr" });
  if (h.herdr && h.herdr_server && h.herdr_server !== h.herdr) out.push({ key: "herdr-server", text: `herdr's server still runs ${h.herdr_server}; ${h.herdr} is installed`, update: "restart-herdr" });
  if (remote && serverVersion && h.remuda && h.remuda !== serverVersion) out.push({ key: "ui", text: `remuda there is ${h.remuda}; this one is ${serverVersion}`, update: "remuda" });
  if (remote && serverVersion && !h.remuda) out.push({ key: "ui", text: "no remuda deployed there — search and usage fall back to slower paths", update: "remuda" });
  const have = remote ? h.remuda : serverVersion;
  if (have && latest?.remuda && newer(have, latest.remuda)) out.push({ key: "release", text: `remuda ${latest.remuda} is out (${have} here)`, update: "remuda-release" });
  if (h.disk_used_pct >= 90 || (h.disk_free_gb > 0 && h.disk_free_gb < 10)) out.push({ key: "disk", text: `Disk ${h.disk_used_pct}% full, ${h.disk_free_gb.toFixed(0)} GB free`, bad: h.disk_used_pct >= 95 });
  if (h.error && !h.claude && !h.herdr) out.push({ key: "err", text: h.error, bad: true });
  return out;
}

// newer: b is a newer x.y.z than a, whatever follows the numbers (a build past a tag, "v0.4.0-24-g5dac31e", is 0.4.0)
export function newer(a: string, b: string): boolean {
  const pa = a.match(/\d+/g)?.slice(0, 3).map(Number) ?? [];
  const pb = b.match(/\d+/g)?.slice(0, 3).map(Number) ?? [];
  if (pa.length < 3 || pb.length < 3) return false;
  for (let i = 0; i < 3; i++) if (pa[i] !== pb[i]) return pb[i]! > pa[i]!;
  return false;
}

function HealthButton({ machine: m, latest, serverVersion }: { machine: Machine; latest?: Latest; serverVersion?: string }) {
  const [open, setOpen] = useState<{ x: number; y: number } | null>(null);
  const [busy, setBusy] = useState(false);
  const [fresh, setFresh] = useState<{ health: Health | null; latest: Latest } | null>(null);
  const [updateWhat, setUpdateWhat] = useState<string | null>(null);
  const close = useCallback(() => setOpen(null), []);
  useDismiss(!!open, close, [".health-pop", `[data-health-btn="${CSS.escape(m.id)}"]`]);
  // a manual read stands until the collector's next one
  useEffect(() => setFresh(null), [m.health?.at]);
  const h = fresh?.health ?? m.health;
  const l = fresh?.latest ?? latest;
  const list = concerns(h, l, serverVersion, m.id !== "local");
  const worst = list.some((c) => c.bad) ? "bad" : list.length ? "warn" : "";
  if (!h && m.state !== "connected") return null;
  const refresh = async () => {
    setBusy(true);
    try {
      setFresh(await refreshHealth(m.id));
    } catch (e) {
      setFresh({ health: { ...(h ?? { at: "", disk_free_gb: 0, disk_used_pct: 0, load1: 0 }), error: errMsg(e) }, latest: l ?? { at: "" } });
    } finally {
      setBusy(false);
    }
  };
  return (
    <>
      <button
        type="button"
        className={`icon-btn${worst ? ` health-${worst}` : ""}${open ? " on" : ""}`}
        data-health-btn={m.id}
        title={list.length ? list.map((c) => c.text).join("\n") : h ? `${m.label}: all good — ${h.os ?? ""}` : "Vitals not read yet"}
        aria-expanded={!!open}
        onClick={(e) => {
          e.stopPropagation();
          const r = e.currentTarget.getBoundingClientRect();
          setOpen(open ? null : { x: Math.min(r.left, window.innerWidth - 300), y: r.bottom + 4 });
        }}
      >
        <Icon name={list.length ? "alert-triangle" : "info"} size={14} />
        {list.length > 0 && <span className="health-count">{list.length}</span>}
      </button>
      {updateWhat !== null && <UpdatesDialog machine={m} what={updateWhat || undefined} onClose={() => setUpdateWhat(null)} onDone={() => void refresh()} />}
      {open && (
        <div className="row-menu health-pop" role="dialog" aria-label={`${m.label} vitals`} style={{ left: open.x, top: open.y }} onClick={(e) => e.stopPropagation()}>
          <div className="health-head">
            <b>{m.label}</b>
            <span className="dim">{h?.os}</span>
            <button type="button" className="icon-btn sm" title="Read again now" disabled={busy} onClick={() => void refresh()}>
              <Icon name="refresh-cw" size={12} className={busy ? "spin" : undefined} />
            </button>
          </div>
          {h ? (
            <dl className="health-rows">
              <dt>Claude Code</dt>
              <dd>
                {h.claude ?? "—"}
                {h.claude && l?.claude && newer(h.claude, l.claude) ? <span className="health-new"> → {l.claude}</span> : null}
              </dd>
              {h.codex && (
                <>
                  <dt>Codex</dt>
                  <dd>
                    {h.codex}
                    {l?.codex && newer(h.codex, l.codex) ? <span className="health-new"> → {l.codex}</span> : null}
                  </dd>
                </>
              )}
              {h.gemini && (
                <>
                  <dt>Gemini CLI</dt>
                  <dd>
                    {h.gemini}
                    {l?.gemini && newer(h.gemini, l.gemini) ? <span className="health-new"> → {l.gemini}</span> : null}
                  </dd>
                </>
              )}
              <dt>herdr</dt>
              <dd>
                {h.herdr ?? "—"}
                {h.herdr_server && h.herdr !== h.herdr_server ? <span className="health-new"> · server {h.herdr_server}</span> : null}
                {h.herdr && l?.herdr && newer(h.herdr, l.herdr) ? <span className="health-new"> → {l.herdr}</span> : null}
              </dd>
              <dt>remuda</dt>
              <dd>
                {h.remuda ?? (m.id === "local" ? serverVersion : "not deployed")}
                {m.id !== "local" && h.remuda ? <span className="dim"> · {h.remuda_running ? "running, terminals go through it" : "not running"}</span> : null}
                {(() => {
                  const have = m.id === "local" ? serverVersion : h.remuda;
                  return have && l?.remuda && newer(have, l.remuda) ? <span className="health-new"> → {l.remuda}</span> : null;
                })()}
              </dd>
              <dt>Disk</dt>
              <dd>
                {h.disk_free_gb.toFixed(0)} GB free · {h.disk_used_pct}% used
              </dd>
              <dt>Load</dt>
              <dd>
                {h.load1.toFixed(2)}
                {h.uptime ? ` · up ${h.uptime}` : ""}
              </dd>
              <dt>Read</dt>
              <dd>{ago(h.at)}</dd>
            </dl>
          ) : (
            <div className="dim">Not read yet.</div>
          )}
          {list.length > 0 && (
            <ul className="health-list">
              {list.map((c) => (
                <li key={c.key} className={c.bad ? "bad" : ""}>
                  <span className="health-concern">{c.text}</span>
                  {c.update && (
                    <button type="button" className="btn sm" onClick={() => { setOpen(null); setUpdateWhat(c.update!); }}>
                      {c.update === "remuda" ? "Deploy" : c.update === "remuda-release" ? "Download & install" : c.update.startsWith("restart-") ? "Restart" : "Update"}
                    </button>
                  )}
                </li>
              ))}
            </ul>
          )}
          <div className="health-foot">
            <button type="button" className="linkish" onClick={() => { setOpen(null); setUpdateWhat(""); }}>
              <Icon name="refresh-cw" size={12} /> All updates…
            </button>
          </div>
          {h?.error && h.claude && <div className="dim">{h.error}</div>}
        </div>
      )}
    </>
  );
}

// ---- the machine's own menu ----
function MachineMenu({ machine: m, onRename, onRemove }: { machine: Machine; onRename: () => void; onRemove?: () => void }) {
  const [open, setOpen] = useState<{ x: number; y: number } | null>(null);
  const [updates, setUpdates] = useState(false);
  const close = useCallback(() => setOpen(null), []);
  useDismiss(!!open, close, [".machine-menu", `[data-machine-menu="${CSS.escape(m.id)}"]`]);
  const pick = (fn: () => void) => () => {
    setOpen(null);
    fn();
  };
  return (
    <>
      <button
        type="button"
        className={`icon-btn${open ? " on" : ""}`}
        data-machine-menu={m.id}
        title="Machine…"
        aria-haspopup="menu"
        aria-expanded={!!open}
        onClick={(e) => {
          e.stopPropagation();
          const r = e.currentTarget.getBoundingClientRect();
          setOpen(open ? null : { x: Math.min(r.left, window.innerWidth - 230), y: r.bottom + 4 });
        }}
      >
        <Icon name="more" size={14} />
      </button>
      {open && (
        <div className="row-menu machine-menu" role="menu" style={{ left: open.x, top: open.y }} onClick={(e) => e.stopPropagation()}>
          <button type="button" role="menuitem" onClick={pick(onRename)}>
            <Icon name="edit" size={14} /> Rename…
          </button>
          <button type="button" role="menuitem" onClick={pick(() => setUpdates(true))} disabled={m.state !== "connected"}>
            <Icon name="refresh-cw" size={14} /> Updates…
          </button>
          {m.host && (
            <button type="button" role="menuitem" onClick={pick(() => void navigator.clipboard?.writeText(m.host!))}>
              <Icon name="copy" size={14} /> Copy SSH host
            </button>
          )}
          {onRemove && (
            <button type="button" role="menuitem" className="danger" onClick={pick(onRemove)}>
              <Icon name="trash" size={14} /> Remove machine
            </button>
          )}
        </div>
      )}
      {updates && <UpdatesDialog machine={m} onClose={() => setUpdates(false)} />}
    </>
  );
}
