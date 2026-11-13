// The rows under a machine, after Orca: every repository a folder — its icon, its name, how its agents stand — that a
// click folds shut or open, and under it a card per workspace — its name or branch, its age, what its agent needs or
// said — that opens the agent in one click, with a line per agent under it (AgentLines.tsx). The orders by priority
// and by ticket prefix draw sections in the folders' place.

import type { KeyboardEvent, MouseEvent } from "react";
import { fillClass } from "@/features/usage/format";
import { FolderIcon, Icon, Menu, Tip, type MenuItem } from "@/ui";
import { VENDOR_NAME } from "@/ui/brand";
import type { Machine } from "./api";
import { branchNamesIssue } from "@/features/jira/jira";
import { CollisionMark, usePeerName } from "./CollisionMark";
import { gravest, markWords } from "./collide";
import { IssueChip } from "./IssueChip";
import { prWords } from "./prStatus";
import { displayOf, stateWords, type Display, type RadarPrefs } from "./radar";
import { hiddenWords } from "./sharing";
import type { Sharing } from "./useSharing";
import type { Section } from "./sections";
import { branchShown, cardWords, entries, folderCounts, folderOf, itemTitle, limited, primaryOf, representative, shortAge, since, type Folder, type WorkspaceItem } from "./workspaces";
import { folderMenuItems, workspaceMenuItems } from "./WorkspaceMenu";
import { AgentLines } from "./AgentLines";
import { HeadSummary, MutedMark, StateMarks } from "./marks";
import { syncTip, syncWords, tallyOf } from "./sidebarAgents";
import { t, tn } from "@/lib/i18n";

export type Current = { machineId: string; sessionId: string; paneId: string } | null;

export type RowProps = {
  machine: Machine;
  current: Current;
  prefs: RadarPrefs;
  step: number; // the spinner's frame
  now: number;
  onSelect: (machineId: string, paneId: string) => void;
  editable?: boolean; // the folders' and the workspaces' menus: none for a viewer
  folded: Record<string, boolean>; // folders and sections folded shut, by "machine|folder:<path>" and "machine|section:<key>"
  onFold: (key: string) => void;
  onIcon?: (m: Machine, folder: Folder) => void; // none for a viewer: the icon is the person's to change
  share: Sharing; // the screen-sharing mode: what reads as a neutral name
  pins: string[]; // the agents pinned to the top, by "machine|pane"
  onPin?: (key: string) => void; // none for a viewer
  subFolded: Record<string, boolean>; // the agents whose subagents are folded, by "machine|pane"
  onFoldSubs: (key: string) => void;
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
        entry.kind === "folder" ? (
          <FolderView key={entry.folder.key} machine={machine} folder={entry.folder} {...rest} />
        ) : entry.kind === "section" ? (
          <SectionView key={entry.section.key} machine={machine} section={entry.section} {...rest} />
        ) : (
          <WorkspaceView key={entry.item.session.id} machine={machine} item={entry.item} {...rest} />
        ),
      )}
    </div>
  );
}

// FolderView is a repository or a folder as Orca shows a project: its icon (glowing while an agent there works), its
// name and how its agents stand; a click folds it shut or open, and open it lists its workspaces' cards. Folded, it
// says how many need you and how many work, so folding never hides a question.
function FolderView({ machine: m, folder, ...rest }: RowProps & { folder: Folder }) {
  const foldKey = `${m.id}|folder:${folder.key}`;
  const isFolded = !!rest.folded[foldKey];
  const n = folderCounts(folder);
  const inside = rest.current?.machineId === m.id && folder.items.some((item) => item.session.id === rest.current?.sessionId);
  const toggle = () => rest.onFold(foldKey);
  const quiet = !n.working && !n.blocked && !n.done;
  const hidden = rest.share.folderHidden(m.id, folder.path);
  const name = hidden ? rest.share.folderName(m.id, folder.path, folder.name) : folder.name;
  const icon = hidden ? undefined : folder.icon;
  const tally = tallyOf(folder.items.map((item) => item.session));
  const primary = primaryOf(folder);
  return (
    <div className={`ws-folder${isFolded ? " folded" : ""}`}>
      <div
        className={`ws-folder-head${isFolded && inside ? " on" : ""}${n.working > 0 ? " is-working" : ""}`}
        role="button"
        tabIndex={0}
        aria-expanded={!isFolded}
        title={`${hidden ? name : folder.path || folder.name}\n${isFolded ? t("Click to show its agents") : t("Click to fold it shut")}`}
        onClick={toggle}
        onKeyDown={(e) => rowKey(e, toggle)}
      >
        <span className={`chevron${isFolded ? "" : " open"}`} aria-hidden="true">
          <Icon name="chevron-right" size={12} />
        </span>
        {rest.onIcon && folder.path && !hidden ? (
          <Tip text={t("Change the folder's icon")}>
            <button
              type="button"
              className="icon-btn sm folder-icon-btn"
              aria-label={t("Change the folder's icon")}
              onClick={(e) => {
                e.stopPropagation();
                rest.onIcon?.(m, folder);
              }}
            >
              <FolderIcon icon={icon} name={name} />
            </button>
          </Tip>
        ) : (
          <FolderIcon icon={icon} name={name} />
        )}
        <span className="ws-folder-name">{name}</span>
        {isFolded && n.fresh > 0 && (
          <span className="ws-badge" title={tn(n.fresh, "{n} file changed since you last reviewed", "{n} files changed since you last reviewed")}>
            {n.fresh}
          </span>
        )}
        <span className="machine-summary">
          {isFolded ? <HeadSummary tally={tally} step={rest.step} /> : <StateMarks working={n.working} blocked={n.blocked} done={n.done} step={rest.step} />}
          {quiet && isFolded && n.agents > 0 && <span className="machine-count">{n.agents}</span>}
        </span>
        {rest.editable && folder.path && (
          <RowMenu className="ws-folder-menu" what={t("Folder actions")} items={() => folderMenuItems(m, folder, { name, masked: hidden })} />
        )}
      </div>
      {!isFolded && (
        <div className="ws-folder-items">
          {folder.items.map((item) => (
            <WorkspaceView key={item.session.id} machine={m} item={item} group={folder.name} primary={item === primary} {...rest} />
          ))}
        </div>
      )}
    </div>
  );
}

// SectionView is one section of the priority or the prefix order: its heading in the colour of the state it gathers,
// how many workspaces it holds, and their cards; it folds like a folder, folded saying how its agents stand
function SectionView({ machine: m, section, ...rest }: RowProps & { section: Section }) {
  const foldKey = `${m.id}|section:${section.key}`;
  const isFolded = !!rest.folded[foldKey];
  const toggle = () => rest.onFold(foldKey);
  const inside = rest.current?.machineId === m.id && section.items.some((item) => item.session.id === rest.current?.sessionId);
  return (
    <div className={`ws-section${isFolded ? " folded" : ""}${section.priority ? ` is-${section.priority}` : ""}`}>
      <div className={`ws-section-head${isFolded && inside ? " on" : ""}`} role="button" tabIndex={0} aria-expanded={!isFolded} onClick={toggle} onKeyDown={(e) => rowKey(e, toggle)}>
        <span className={`chevron${isFolded ? "" : " open"}`} aria-hidden="true">
          <Icon name="chevron-right" size={12} />
        </span>
        <span className="ws-section-name">{section.title}</span>
        <span className="badge quiet">{section.items.length}</span>
        {isFolded && !section.priority && (
          <span className="machine-summary">
            <HeadSummary tally={tallyOf(section.items.map((item) => item.session))} step={rest.step} />
          </span>
        )}
      </div>
      {!isFolded && (
        <div className="ws-section-items">
          {section.items.map((item) => (
            <WorkspaceView key={item.session.id} machine={m} item={item} compact={section.priority === "needs"} {...rest} />
          ))}
        </div>
      )}
    </div>
  );
}

// WorkspaceView is one workspace: the two-line card that opens its agent, and under it a line per agent
function WorkspaceView({ machine: m, item, group, primary, compact, current, prefs, step, now, onSelect, editable, share, ...lines }: RowProps & { item: WorkspaceItem; group?: string; primary?: boolean; compact?: boolean }) {
  const s = item.session;
  const lead = item.lead;
  const pane = lead?.pane;
  const agent = pane?.agent ?? "";
  const d: Display = agent && pane ? displayOf(pane, prefs, now) : "idle";
  const alert = pane?.radar?.alert;
  const selected = current?.machineId === m.id && current.sessionId === s.id;
  const several = item.rows.length > 1;
  const agents = item.rows.some((r) => r.pane.agent);
  const hidden = share.sessionHidden(m.id, s);
  const peerName = usePeerName();
  const collision = gravest(s.collisions);
  // a hidden folder's card goes by what runs in it, the folder's neutral name standing above it
  const title = hidden ? (group ? (agent ? (VENDOR_NAME[agent] ?? agent) : t("terminal")) : share.folderName(m.id, folderOf(s), s.name)) : itemTitle(item, group);
  const branch = hidden ? "" : branchShown(item, title);
  const words = hidden ? (agents ? "" : hiddenWords()) : cardWords(item);
  const sync = selected && !hidden ? syncWords(s.sync) : "";
  const target = agent && pane ? pane.id : (representative(s)?.id ?? "");
  const open = () => target && onSelect(m.id, target);
  const usage = !several ? pane?.usage : undefined;
  const fill = usage && usage.window > 0 ? usage.context / usage.window : 0;
  const folder = s.dir || s.worktree?.checkout_path || pane?.cwd || "";
  const state = agent && pane ? `${agent} · ${hidden ? stateWords(d, pane.radar?.last_active, now, pane.radar?.since) : alert || stateWords(d, pane.radar?.last_active, now, pane.radar?.since)}` : "";
  const tip = hidden
    ? [title, state].filter(Boolean).join("\n")
    : [
        `${s.name}${s.branch ? ` · ${s.branch}` : ""}`,
        state,
        !several && pane?.said ? pane.said : "",
        pane?.pr ? `${pane.pr.label}${pane.pr.status ? ` · ${prWords(pane.pr.status)}` : ""} · ${pane.pr.url}` : "",
        collision ? markWords(collision, peerName(collision, m.id).full) : "",
        folder,
      ].filter(Boolean).join("\n");
  const sub = branch || primary || sync || words;
  return (
    <div className={`ws-item is-${agent ? d : "shell"}${selected ? " on" : ""}${alert ? " is-alert" : ""}${!several && limited(lead) ? " is-limit" : ""}${group ? " in-repo" : ""}`}>
      <div className="ws-main" role="button" tabIndex={0} onClick={open} onKeyDown={(e) => rowKey(e, open)} title={tip}>
        <span className="ws-line">
          {!agents && <span className="radar-mark">$</span>}
          <span className="ws-name">{title}</span>
          {s.review?.fresh ? (
            <span className="ws-badge" title={tn(s.review.fresh, "{n} file changed since you last reviewed", "{n} files changed since you last reviewed")}>
              {s.review.fresh}
            </span>
          ) : null}
          {!hidden && <CollisionMark machineId={m.id} session={s} className="ws-collide" compact />}
          <MutedMark session={s} />
          {fill >= 0.5 && usage && (
            <span className={`radar-ctx${fillClass(fill)}`} title={t("Context {pct}% full · {model} · ${cost}", { pct: Math.round(100 * fill), model: usage.name, cost: usage.cost.toFixed(2) })}>
              {Math.round(100 * fill)}%
            </span>
          )}
          <span className="ws-age">{shortAge(since(lead), now)}</span>
          {editable && <RowMenu className="ws-menu-btn" what={t("Workspace actions")} items={() => workspaceMenuItems(m, s, { name: hidden ? share.folderName(m.id, folderOf(s), s.name) : "", masked: hidden })} />}
        </span>
        {sub && (
          <span className="ws-sub">
            {branch && !branchNamesIssue(branch, s.jira?.[0]) && (
              <span className="ws-branch">
                <Icon name="git-branch" size={12} />
                <span>{branch}</span>
              </span>
            )}
            {primary && !hidden && (
              <span className="primary-tag" title={t("The repository's main checkout; its worktrees are listed beside it")}>
                {t("primary")}
              </span>
            )}
            {sync && (
              <span className="ws-sync" title={syncTip(s.sync)}>
                {sync}
              </span>
            )}
            {!hidden && s.jira?.[0] && <IssueChip issue={s.jira[0]} />}
            {words && <span className="ws-words">{words}</span>}
          </span>
        )}
      </div>
      <AgentLines
        rows={item.rows}
        machine={m}
        prefs={prefs}
        step={step}
        now={now}
        selectedPane={current?.machineId === m.id ? current.paneId : null}
        onSelect={onSelect}
        pins={lines.pins}
        onPin={lines.onPin}
        subFolded={lines.subFolded}
        onFoldSubs={lines.onFoldSubs}
        hidden={hidden}
        compact={compact}
      />
    </div>
  );
}

// RowMenu is a folder's or a workspace's ⋯: a menu that opens inside the window, its button's click kept from the row
function RowMenu({ className, what, items }: { className: string; what: string; items: () => MenuItem[] }) {
  const keep = (e: MouseEvent<HTMLButtonElement>) => e.stopPropagation();
  return (
    <Menu
      label={what}
      tip={what}
      items={items}
      trigger={
        <button type="button" className={`icon-btn ${className}`} aria-label={what} onClick={keep}>
          <Icon name="more" size={14} />
        </button>
      }
    />
  );
}
