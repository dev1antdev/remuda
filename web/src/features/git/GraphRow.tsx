// One row of the graph, after git-graph's: the lanes (a fork or a merge a curve, the dot a ring at HEAD), the labels and
// the subject clipped together so the date never moves, then the date, the author and the commit. Memoised: a poll that
// moved nothing redraws no row.

import { memo, type CSSProperties } from "react";
import { Icon, Tip } from "@/ui";
import type { IconName } from "@/ui/icons";
import type { Commit } from "./api";
import { isHead, labelsOf, type Label } from "./labels";
import type { LaneRow } from "./lanes";
import { dateWords } from "./words";
import { t } from "@/lib/i18n";

export const ROW = 24;
export const LANE = 12;
export const PAD = 6;

const x = (lane: number) => PAD + lane * LANE + LANE / 2;

// lanesWidth is the graph column's width for so many lanes, before the 40 % it is clipped to
export const lanesWidth = (lanes: number) => PAD * 2 + Math.max(1, lanes) * LANE;

function Lanes({ row, width, colors, ring, faint }: { row: LaneRow; width: number; colors: string[]; ring: boolean; faint: boolean }) {
  const colour = (lane: number) => colors[lane % colors.length]!;
  const dot = faint ? "var(--fg-faint)" : colour(row.lane);
  return (
    <svg className="git-lanes" width={width} height={ROW} viewBox={`0 0 ${width} ${ROW}`} aria-hidden="true">
      {row.segments.map((s, i) => {
        const ya = s.fromY * ROW;
        const yb = s.toY * ROW;
        const mid = (ya + yb) / 2;
        const d = s.from === s.to ? `M${x(s.from)} ${ya}L${x(s.to)} ${yb}` : `M${x(s.from)} ${ya}C${x(s.from)} ${mid} ${x(s.to)} ${mid} ${x(s.to)} ${yb}`;
        return <path key={i} d={d} fill="none" stroke={s.dashed ? "var(--fg-faint)" : colour(s.color)} strokeWidth={1.5} strokeDasharray={s.dashed ? "3 3" : undefined} />;
      })}
      {ring ? <circle cx={x(row.lane)} cy={ROW / 2} r={3.25} fill="var(--bg)" stroke={dot} strokeWidth={2} /> : <circle cx={x(row.lane)} cy={ROW / 2} r={4} fill={dot} />}
    </svg>
  );
}

const ICON: Record<Label["kind"], IconName | null> = { branch: "git-branch", remote: "cloud", tag: "tag", head: null };

// Chip is one label; each part carries what a press on it names, for the list's own handlers
function Chip({ label, colour }: { label: Label; colour: string }) {
  const style = { "--lane": label.kind === "tag" ? "var(--amber)" : label.kind === "head" ? "var(--accent)" : colour } as CSSProperties;
  return (
    <span className={`git-label is-${label.kind}${label.head ? " is-current" : ""}`} style={style}>
      {label.parts.map((part, i) => (
        <span key={part.name + i} className={`git-label-part${i > 0 ? " is-rider" : ""}`} data-ref-kind={part.kind} data-ref-name={part.name} title={part.text !== part.name ? part.name : undefined}>
          {part.icon && <Icon name={label.head && part.kind === "branch" ? "check" : ICON[part.kind]!} size={12} />}
          {part.text}
        </span>
      ))}
    </span>
  );
}

export const GraphRow = memo(function GraphRow({ index, commit, row, width, colors, remotes, selected, now, menu, dirty }: {
  index: number;
  commit: Commit | null; // null for the uncommitted row
  row: LaneRow;
  width: number;
  colors: string[];
  remotes: boolean;
  selected: boolean;
  now: number;
  menu: boolean; // a ⋯ at the row's end, on a touch screen with the menus there
  dirty: number; // what the uncommitted row counts
}) {
  const colour = colors[row.lane % colors.length]!;
  const head = !!commit && isHead(commit);
  const merge = !!commit && commit.parents.length > 1;
  const style = { "--lane": colour } as CSSProperties;
  return (
    <div className={`git-row${head ? " is-head" : ""}${merge ? " is-merge" : ""}${!commit ? " is-uncommitted" : ""}${selected ? " on" : ""}`} style={style} data-index={index} role="row" aria-selected={selected}>
      <span className="git-graph" role="cell">
        <Lanes row={row} width={width} colors={colors} ring={head || !commit} faint={!commit} />
      </span>
      <span className="git-desc" role="cell">
        {commit && labelsOf(commit, remotes).map((label, i) => <Chip key={i} label={label} colour={colour} />)}
        <span className="git-subject">{commit ? commit.subject : t("Uncommitted changes ({n})", { n: dirty })}</span>
      </span>
      <span className="git-date" role="cell">
        {commit ? dateWords(commit.time, now) : "*"}
      </span>
      <span className="git-author" role="cell">
        {commit ? commit.author : "*"}
      </span>
      <span className="git-hash" role="cell">
        {commit ? commit.hash.slice(0, 8) : "*"}
      </span>
      {menu && (
        <Tip text={commit ? t("What to do with this commit") : t("What to do with the uncommitted changes")}>
          <button type="button" className="icon-btn sm git-row-more" data-more="" aria-label={commit ? t("What to do with this commit") : t("What to do with the uncommitted changes")}>
            <Icon name="more" size={14} />
          </button>
        </Tip>
      )}
    </div>
  );
});
