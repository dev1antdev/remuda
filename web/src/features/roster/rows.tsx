// The rows under a machine, after Orca: one line per workspace — its agent's mark and logo, its name, its age, and under
// it the branch and what the agent does — that opens the agent in one click; a small line per agent when it runs
// more than one; the worktrees of a repository gathered under the repository's name.

import type { CSSProperties, KeyboardEvent, MouseEvent } from "react";
import { fillClass } from "@/features/usage/format";
import { Icon, Logo } from "@/ui";
import { brandColor } from "@/ui/brand";
import type { Machine, Session } from "./api";
import { muteWords, mutedUntil } from "./mute";
import { prWords } from "./prStatus";
import { displayOf, markOf, rowLabel, stateWords, type AgentRow, type Display, type RadarPrefs } from "./radar";
import { agentWords, branchShown, entries, itemTitle, itemWords, representative, shortAge, since, type Entry, type WorkspaceItem } from "./workspaces";
import { t, tn } from "@/lib/i18n";

export type Current = { machineId: string; sessionId: string; paneId: string } | null;

export type RowProps = {
  machine: Machine;
  current: Current;
  prefs: RadarPrefs;
  step: number; // the spinner's frame
  now: number;
  onSelect: (machineId: string, paneId: string) => void;
  onMenu?: (m: Machine, s: Session, x: number, y: number) => void; // none for a viewer: no workspace actions
  folded: Record<string, boolean>; // repositories folded shut, by "machine|repo:<key>"
  onFold: (key: string) => void;
};

// rowKey makes a div row a button: Enter or Space on the row itself picks it, on a button inside it they are that button's
function rowKey(e: KeyboardEvent<HTMLDivElement>, pick: () => void) {
  if (e.target !== e.currentTarget || (e.key !== "Enter" && e.key !== " ")) return;
  e.preventDefault();
  pick();
}

export function Rows({ machine, ...rest }: RowProps) {
  return (
    <div className="ws-tree">
      {entries(machine, rest.prefs).map((entry) =>
        entry.kind === "repo" ? <RepoView key={entry.key} machine={machine} entry={entry} {...rest} /> : <WorkspaceView key={entry.item.session.id} machine={machine} item={entry.item} {...rest} />,
      )}
    </div>
  );
}

// RepoView is a repository with worktrees open: its name, and its checkouts under it unless folded shut
function RepoView({ machine: m, entry, ...rest }: RowProps & { entry: Extract<Entry, { kind: "repo" }> }) {
  const foldKey = `${m.id}|${entry.key}`;
  const isFolded = !!rest.folded[foldKey];
  const counts = { working: 0, blocked: 0, done: 0 };
  for (const item of entry.items)
    for (const r of item.rows) {
      const state = r.pane.agent ? r.pane.radar?.state : undefined;
      if (state === "working" || state === "blocked" || state === "done") counts[state]++;
    }
  const inside = rest.current?.machineId === m.id && entry.items.some((item) => item.session.id === rest.current?.sessionId);
  return (
    <div className={`ws-repo${isFolded ? " folded" : ""}`}>
      <div
        className={`ws-repo-head${isFolded && inside ? " on" : ""}`}
        role="button"
        tabIndex={0}
        aria-expanded={!isFolded}
        title={isFolded ? t("Show its worktrees") : t("Hide its worktrees")}
        onClick={() => rest.onFold(foldKey)}
        onKeyDown={(e) => rowKey(e, () => rest.onFold(foldKey))}
      >
        <span className={`chevron${isFolded ? "" : " open"}`} aria-hidden="true">
          <Icon name="chevron-right" size={12} />
        </span>
        <Icon name="git-branch" size={12} />
        <span className="ws-repo-name">{entry.name}</span>
        {isFolded ? (
          <span className="machine-summary">
            <StateMarks working={counts.working} blocked={counts.blocked} done={counts.done} step={rest.step} />
          </span>
        ) : (
          <span className="machine-count">{tn(entry.items.length, "{n} checkout", "{n} checkouts")}</span>
        )}
      </div>
      {!isFolded && (
        <div className="ws-repo-items">
          {entry.items.map((item) => (
            <WorkspaceView key={item.session.id} machine={m} item={item} group={entry.name} {...rest} />
          ))}
        </div>
      )}
    </div>
  );
}

// WorkspaceView is one workspace: the two-line card that opens its agent, and the agents' own lines when it runs several
function WorkspaceView({ machine: m, item, group, current, prefs, step, now, onSelect, onMenu }: RowProps & { item: WorkspaceItem; group?: string }) {
  const s = item.session;
  const lead = item.lead;
  const pane = lead?.pane;
  const agent = pane?.agent ?? "";
  const d: Display = agent && pane ? displayOf(pane, prefs, now) : "idle";
  const alert = pane?.radar?.alert;
  const selected = current?.machineId === m.id && current.sessionId === s.id;
  const title = itemTitle(item, group);
  const branch = branchShown(item, title);
  const words = itemWords(item, prefs);
  const several = item.rows.length > 1;
  const target = agent && pane ? pane.id : (representative(s)?.id ?? "");
  const open = () => target && onSelect(m.id, target);
  const markStyle: CSSProperties | undefined = d === "working" && agent && !alert ? { color: brandColor(agent) } : undefined;
  const usage = !several ? pane?.usage : undefined;
  const fill = usage && usage.window > 0 ? usage.context / usage.window : 0;
  const folder = s.dir || s.worktree?.checkout_path || pane?.cwd || "";
  const tip = [
    `${s.name}${s.branch ? ` · ${s.branch}` : ""}`,
    agent && pane ? `${agent} · ${alert || stateWords(d, pane.radar?.last_active, now, pane.radar?.since)}` : "",
    !several && pane?.said ? pane.said : "",
    pane?.pr ? `${pane.pr.label}${pane.pr.status ? ` · ${prWords(pane.pr.status)}` : ""} · ${pane.pr.url}` : "",
    folder,
  ].filter(Boolean).join("\n");
  return (
    <div className={`ws-item is-${agent ? d : "shell"}${selected ? " on" : ""}${alert ? " is-alert" : ""}${group ? " in-repo" : ""}`}>
      <div className="ws-main" role="button" tabIndex={0} onClick={open} onKeyDown={(e) => rowKey(e, open)} title={tip}>
        <span className="ws-line">
          <span className="radar-mark" style={markStyle}>
            {alert ? "!" : agent ? markOf(d, step) : "$"}
          </span>
          {agent && <Logo agent={agent} />}
          <span className="ws-name">{title}</span>
          {s.review?.fresh ? (
            <span className="ws-badge" title={tn(s.review.fresh, "{n} file changed since you last reviewed", "{n} files changed since you last reviewed")}>
              {s.review.fresh}
            </span>
          ) : null}
          <MutedMark session={s} />
          {fill >= 0.5 && usage && (
            <span className={`radar-ctx${fillClass(fill)}`} title={t("Context {pct}% full · {model} · ${cost}", { pct: Math.round(100 * fill), model: usage.name, cost: usage.cost.toFixed(2) })}>
              {Math.round(100 * fill)}%
            </span>
          )}
          <span className="ws-age">{shortAge(since(lead), now)}</span>
          {onMenu && <MenuButton machine={m} session={s} onMenu={onMenu} />}
        </span>
        <span className="ws-sub">
          {branch && (
            <span className="ws-branch">
              <Icon name="git-branch" size={12} />
              <span>{branch}</span>
            </span>
          )}
          <span className="ws-words">{words}</span>
        </span>
      </div>
      {several && (
        <div className="ws-agents">
          {item.rows.map((r) => (
            <AgentLine key={r.pane.id} machine={m} row={r} selected={current?.machineId === m.id && current.paneId === r.pane.id} prefs={prefs} step={step} now={now} onSelect={onSelect} />
          ))}
        </div>
      )}
    </div>
  );
}

// AgentLine is one of several agents in a workspace: its mark, logo, what it does and its age, a click opening it
function AgentLine({ machine: m, row: r, selected, prefs, step, now, onSelect }: { machine: Machine; row: AgentRow; selected: boolean; prefs: RadarPrefs; step: number; now: number; onSelect: RowProps["onSelect"] }) {
  const p = r.pane;
  const d: Display = p.agent ? displayOf(p, prefs, now) : "idle";
  const alert = p.radar?.alert;
  const style = d === "working" && p.agent && !alert ? { color: brandColor(p.agent) } : undefined;
  const open = () => onSelect(m.id, p.id);
  const label = rowLabel(p, r.tab, r.session.name, prefs);
  const words = p.agent ? agentWords(r, prefs) : label;
  return (
    <div
      className={`ws-agent-line is-${p.agent ? d : "shell"}${selected ? " on" : ""}${alert ? " is-alert" : ""}`}
      role="button"
      tabIndex={0}
      onClick={open}
      onKeyDown={(e) => rowKey(e, open)}
      title={`${p.agent || "shell"} · ${alert || stateWords(d, p.radar?.last_active, now, p.radar?.since)}\n${label}${words !== label ? `\n${words}` : ""}`}
    >
      <span className="radar-mark" style={style}>
        {alert ? "!" : p.agent ? markOf(d, step) : "$"}
      </span>
      {p.agent && <Logo agent={p.agent} />}
      <span className="ws-agent-words">{words}</span>
      <span className="ws-age">{shortAge(since(r), now)}</span>
    </div>
  );
}

function MenuButton({ machine, session, onMenu }: { machine: Machine; session: Session; onMenu: NonNullable<RowProps["onMenu"]> }) {
  const open = (e: MouseEvent<HTMLButtonElement>) => {
    e.stopPropagation();
    const r = e.currentTarget.getBoundingClientRect();
    onMenu(machine, session, r.right, r.bottom);
  };
  return (
    <button type="button" className="icon-btn ws-menu-btn" title={t("Workspace actions")} aria-label={t("Workspace actions")} aria-haspopup="menu" onClick={open}>
      <Icon name="more" size={14} />
    </button>
  );
}

// MutedMark is the small bell-off a muted workspace wears, with how long the silence holds in its tooltip
export function MutedMark({ session }: { session: Session }) {
  const until = mutedUntil(session);
  if (!until) return null;
  return (
    <span className="radar-muted" title={muteWords(until)} aria-label={muteWords(until)}>
      <Icon name="bell-off" size={12} />
    </span>
  );
}

// StateMarks are a folded machine's or repository's counts: how many work, ask, finished
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
