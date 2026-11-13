// The dialog's source: the Branch · Issue · PR · New tabs, the base a new branch starts from and a fetch from origin on
// their right, then the search field and its rows, each with what it is (Open, Local, Remote, Issue, PR).

import type { KeyboardEvent, RefObject } from "react";
import { Icon, Select, Tip, type IconName } from "@/ui";
import { t, tc } from "@/lib/i18n";
import { MODES, type Item, type Mode } from "./sources";

const MODE_WORDS: Record<Mode, { label: string; icon: IconName; hint: string }> = {
  branch: { label: "Branch", icon: "git-branch", hint: "Filter branches, including ones only on origin" },
  issue: { label: "Issue", icon: "circle-dot", hint: "Filter open issues, or paste #123 or a link" },
  pr: { label: "PR", icon: "git-pull-request", hint: "Filter open pull requests, or paste #123 or a link" },
  new: { label: "New", icon: "plus", hint: "" },
};

const ROW_ICON: Record<Item["kind"], IconName> = { open: "folder", local: "git-branch", remote: "cloud", issue: "circle-dot", pr: "git-pull-request", lookup: "external-link" };

export function SourceTabs({ mode, onMode, bases, base, onBase, showBase, fetching, onFetch }: {
  mode: Mode;
  onMode: (m: Mode) => void;
  bases: string[];
  base: string;
  onBase: (b: string) => void;
  showBase: boolean; // the plan makes a new branch from the base: the pick shows; else it keeps its room unseen
  fetching: boolean;
  onFetch: () => void;
}) {
  return (
    <div className="nw-tabs-row">
      <div className="nw-tabs" role="tablist" aria-label={t("Start from")}>
        {MODES.map((m) => (
          <button key={m} type="button" role="tab" aria-selected={mode === m} className={`btn ghost sm${mode === m ? " on" : ""}`} data-mode={m} onClick={() => onMode(m)}>
            <Icon name={MODE_WORDS[m].icon} size={14} />
            {tc("worktree-tab", MODE_WORDS[m].label)}
          </button>
        ))}
      </div>
      <div className={`nw-base${showBase ? "" : " is-vacant"}`} inert={!showBase || undefined}>
        <span>{t("Create from")}</span>
        <Select value={base} onChange={onBase} options={bases.map((b) => ({ value: b, label: b, icon: <Icon name="git-branch" size={14} /> }))} className="sm" />
      </div>
      <Tip text={t("Fetch from origin: git fetch --prune origin")}>
        <button type="button" className="icon-btn" aria-label={t("Fetch from origin")} disabled={fetching} onClick={onFetch}>
          <Icon name="refresh-cw" size={14} className={fetching ? "spin" : undefined} />
        </button>
      </Tip>
    </div>
  );
}

export function SourceList({ mode, query, onQuery, rows, at, onPick, empty, field, onKey }: {
  mode: Mode;
  query: string;
  onQuery: (q: string) => void;
  rows: Item[];
  at: number;
  onPick: (i: number) => void;
  empty: string;
  field: RefObject<HTMLInputElement | null>;
  onKey: (e: KeyboardEvent<HTMLInputElement>) => void;
}) {
  return (
    <div className="nw-source">
      <div className="nw-search">
        <Icon name="search" size={14} />
        <input ref={field} className="bare" value={query} placeholder={t(MODE_WORDS[mode].hint)} aria-label={t(MODE_WORDS[mode].hint)} spellCheck={false} autoComplete="off" onChange={(e) => onQuery(e.target.value)} onKeyDown={onKey} />
      </div>
      <div className="nw-rows" role="listbox" aria-label={t(MODE_WORDS[mode].label)}>
        {rows.map((row, i) => (
          <button
            key={row.key}
            type="button"
            role="option"
            aria-selected={i === at}
            className={`btn ghost nw-row${i === at ? " on" : ""}`}
            data-tag={row.tag}
            onMouseDown={(e) => e.preventDefault()}
            onClick={() => onPick(i)}
            ref={(el) => {
              if (el && i === at) el.scrollIntoView({ block: "nearest" });
            }}
          >
            <Icon name={ROW_ICON[row.kind]} size={14} className={`nw-row-icon tone-${row.tag.toLowerCase()}`} />
            <span className="nw-row-text">
              <span className={`nw-row-label${row.mono ? " mono" : ""}`}>{row.label}</span>
              <span className="nw-row-sub">{row.sub}</span>
            </span>
            <span className={`nw-tag tone-${row.tag.toLowerCase()}`}>{tc("worktree-tag", row.tag)}</span>
          </button>
        ))}
        {rows.length === 0 && <p className="nw-empty">{empty}</p>}
      </div>
    </div>
  );
}
