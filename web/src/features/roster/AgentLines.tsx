// The agents under a workspace's card, after Orca's better-sidebar: a line per agent — its logo, its title (muted at
// rest), the time in its state, and its shape — that opens it in one click, a pin that takes the age's place under the
// pointer, and the subagents at work under it on thin tree lines, which a chevron folds, the count left in their colour.

import type { CSSProperties, KeyboardEvent, MouseEvent } from "react";
import { actions } from "@/app/store";
import { Icon, Logo, Tip } from "@/ui";
import { VENDOR_NAME, brandColor } from "@/ui/brand";
import type { Machine, Pane } from "./api";
import { agentKey, elbowOf, shapeOf, subagentCount, subagentsOf, subagentsWord } from "./sidebarAgents";
import { AgentShape } from "./marks";
import { displayOf, rowLabel, stateWords, type AgentRow, type RadarPrefs } from "./radar";
import { hiddenWords } from "./sharing";
import { agentWords, shortAge, since } from "./workspaces";
import { t, tn } from "@/lib/i18n";

// LineProps is what every agent line needs beside its row: the clocks, the selection, the pins and the folds
export type LineProps = {
  machine: Machine;
  prefs: RadarPrefs;
  step: number;
  now: number;
  selectedPane: string | null; // the pane on screen when it is this machine's
  onSelect: (machineId: string, paneId: string) => void;
  pins: string[];
  onPin?: (key: string) => void; // none for a viewer, whose pins would not follow it anywhere
  subFolded: Record<string, boolean>;
  onFoldSubs: (key: string) => void;
  hidden: boolean; // the screen-sharing mode hides the workspace's folder
  compact?: boolean; // under a heading that already says "Needs you": the question is the "!" square
  where?: string; // the pinned group names the workspace and machine after the title
};

// rowKey makes a div row a button: Enter or Space on the row itself picks it, on a button inside it they are that button's
function rowKey(e: KeyboardEvent<HTMLDivElement>, pick: () => void) {
  if (e.target !== e.currentTarget || (e.key !== "Enter" && e.key !== " ")) return;
  e.preventDefault();
  pick();
}

const keep = (e: MouseEvent) => e.stopPropagation();

// AgentLines is a workspace's agents, each a line with its subagents under it
export function AgentLines({ rows, ...props }: LineProps & { rows: AgentRow[] }) {
  if (rows.length === 0) return null;
  return (
    <div className="ws-agents">
      {rows.map((r) => (
        <AgentLine key={r.pane.id} row={r} {...props} />
      ))}
    </div>
  );
}

// AgentLine is one agent: a click opens it; its subagents follow unless folded
export function AgentLine({ row: r, machine: m, prefs, step, now, selectedPane, onSelect, pins, onPin, subFolded, onFoldSubs, hidden, compact, where }: LineProps & { row: AgentRow }) {
  const p = r.pane;
  const key = agentKey(m.id, p.id);
  const d = p.agent ? displayOf(p, prefs, now) : "idle";
  const shape = shapeOf(p, d);
  const pinned = pins.includes(key);
  const subs = p.agent ? subagentCount(p) : 0;
  const folded = !!subFolded[key];
  const label = hidden ? (p.agent ? (VENDOR_NAME[p.agent] ?? p.agent) : t("terminal")) : rowLabel(p, r.tab, r.session.name, prefs); // a hidden folder's agent goes by its kind
  const state = p.agent ? stateWords(d, p.radar?.last_active, now, p.radar?.since) : t("shell");
  const words = hidden || !p.agent ? "" : agentWords(r, prefs);
  const tip = [`${p.agent || t("shell")} · ${p.radar?.alert || state}`, label, words !== label ? words : "", where ?? ""].filter(Boolean).join("\n");
  const open = () => onSelect(m.id, p.id);
  const spin: CSSProperties | undefined = shape === "working" && p.agent ? { color: brandColor(p.agent) } : undefined;
  return (
    <>
      <div
        className={`ws-agent-line is-${p.agent ? d : "shell"} shape-${shape}${selectedPane === p.id ? " on" : ""}${pinned ? " is-pinned" : ""}`}
        role="button"
        tabIndex={0}
        onClick={open}
        onKeyDown={(e) => rowKey(e, open)}
        title={tip}
      >
        {p.agent ? <Logo agent={p.agent} /> : <span className="radar-mark">$</span>}
        <span className="ws-agent-words">{label}</span>
        {where && <span className="agent-line-where">{where}</span>}
        {subs > 0 && <SubagentsFold paneKey={key} count={subs} folded={folded} onFold={onFoldSubs} />}
        <span className="ws-age">{p.agent ? shortAge(since(r), now) : ""}</span>
        {onPin && p.agent && <PinButton pinned={pinned} onPin={() => onPin(key)} />}
        {p.agent && <AgentShape shape={shape} step={step} compact={compact} style={spin} />}
      </div>
      {subs > 0 && !folded && <SubagentTree machine={m} pane={p} hidden={hidden} step={step} />}
    </>
  );
}

// SubagentsFold folds an agent's subagents, and folded says how many work, in the colour of work
function SubagentsFold({ paneKey, count, folded, onFold }: { paneKey: string; count: number; folded: boolean; onFold: (key: string) => void }) {
  const what = folded ? tn(count, "Show its subagent", "Show its {n} subagents") : t("Fold its subagents");
  return (
    <Tip text={what}>
      <button
        type="button"
        className={`icon-btn sm subagents-fold${folded ? " is-folded" : ""}`}
        aria-label={what}
        aria-expanded={!folded}
        onClick={(e) => {
          keep(e);
          onFold(paneKey);
        }}
      >
        <Icon name={folded ? "chevron-right" : "chevron-down"} size={12} />
        {folded && <span className="subagents-count">{count}</span>}
      </button>
    </Tip>
  );
}

// PinButton pins the agent to the group at the top of the sidebar, or unpins it
function PinButton({ pinned, onPin }: { pinned: boolean; onPin: () => void }) {
  const what = pinned ? t("Unpin this agent") : t("Pin this agent to the top");
  return (
    <Tip text={what}>
      <button
        type="button"
        className={`icon-btn sm agent-pin${pinned ? " on" : ""}`}
        aria-label={what}
        aria-pressed={pinned}
        onClick={(e) => {
          keep(e);
          onPin();
        }}
      >
        <Icon name={pinned ? "pin-off" : "pin"} size={12} />
      </button>
    </Tip>
  );
}

// SubagentTree is an agent's subagents at work, the oldest first, each on an elbow of the tree; a click opens the
// transcript viewer on its own conversation. A hidden folder's say nothing of what they do.
function SubagentTree({ machine: m, pane: p, hidden, step }: { machine: Machine; pane: Pane; hidden: boolean; step: number }) {
  const { list, more } = subagentsOf(p);
  const count = list.length + (more > 0 ? 1 : 0);
  const session = p.subagents?.session ?? "";
  return (
    <div className="sub-tree" aria-label={subagentsWord(p.subagents?.working ?? list.length)}>
      {list.map((sub, i) => {
        const open = () => session && actions.openViewer({ machine: m.id, dir: p.cwd, session, subagent: { id: sub.id, description: sub.description || sub.id } });
        const name = hidden ? hiddenWords() : sub.description || t("a subagent");
        return (
          <div
            key={sub.id}
            className="sub-line"
            role="button"
            tabIndex={0}
            onClick={open}
            onKeyDown={(e) => rowKey(e, open)}
            title={hidden ? name : [name, sub.doing ?? "", t("Open its conversation")].filter(Boolean).join("\n")}
          >
            <span className={`sub-elbow${elbowOf(i, count) === "├" ? " goes-on" : ""}`} aria-hidden="true" />
            <span className="sub-name">{name}</span>
            <AgentShape shape="working" step={step} />
          </div>
        );
      })}
      {more > 0 && (
        <div className="sub-line is-more">
          <span className="sub-elbow" aria-hidden="true" />
          <span className="sub-name">{tn(more, "{n} more at work", "{n} more at work")}</span>
        </div>
      )}
    </div>
  );
}
