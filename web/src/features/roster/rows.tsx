// The rows under a machine: a workspace's line with its agents under it (worktrees nested), or one flat
// list by activity. One row per agent: a mark for what it is doing, its vendor's logo, its title in the state's colour.

import type { KeyboardEvent } from "react";
import { fillClass } from "@/features/usage/format";
import { basename } from "@/lib/text";
import { Icon, Logo, StatusDot } from "@/ui";
import { brandColor } from "@/ui/brand";
import type { Machine, Session } from "./api";
import { agentRows, displayOf, groups, markOf, recentRows, rowLabel, stateWords, type AgentRow, type Display, type Group, type RadarPrefs } from "./radar";

export type Current = { machineId: string; sessionId: string; paneId: string } | null;

export type RowProps = {
  machine: Machine;
  current: Current;
  prefs: RadarPrefs;
  step: number; // the spinner's frame
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

export function Rows({ machine, ...rest }: RowProps) {
  return (
    <div className="ws-tree">
      {rest.prefs.order === "recent" ? <Flat machine={machine} {...rest} /> : groups(machine, rest.prefs).map((g) => <GroupView key={g.session.id} machine={machine} group={g} nested={false} {...rest} />)}
    </div>
  );
}

type GroupSummary = { working: number; blocked: number; done: number; agents: number; children: number };

// groupSummary is what a folded group still says: its agents' states, and its worktrees
function groupSummary(g: Group): GroupSummary {
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
  summary?: GroupSummary | null; // set while folded
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
          <StateMarks working={summary.working} blocked={summary.blocked} done={summary.done} step={step} />
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

// StateMarks are a folded machine's or group's counts: how many work, ask, finished
export function StateMarks({ working, blocked, done, step }: { working: number; blocked: number; done: number; step: number }) {
  return (
    <>
      {working > 0 && (
        <span className="radar-mark working">
          {markOf("working", step)} {working}
        </span>
      )}
      {blocked > 0 && <span className="radar-mark blocked">? {blocked}</span>}
      {done > 0 && (
        <span className="radar-mark done">
          <Icon name="check" size={12} /> {done}
        </span>
      )}
    </>
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

// representative is the pane a workspace's own line stands for: the focused one, else an agent's, else the first
function representative(s: Session) {
  const tab = s.tabs.find((t) => t.id === s.active_tab_id) ?? s.tabs[0];
  if (!tab) return null;
  return tab.panes.find((p) => p.focused) ?? tab.panes.find((p) => p.agent) ?? tab.panes[0] ?? null;
}
