// The issues table: the Open and Assigned to me chips, the Filters menu and the search field that both rewrite the
// visible query, the rows 25 a page, and a foot saying how many and how old the read is.

import { useEffect, useState, type ReactNode } from "react";
import { useNow } from "@/lib/clock";
import { agoWords } from "@/lib/time";
import { Icon, Menu, Tip, type MenuItem } from "@/ui";
import { t, tc, tn } from "@/lib/i18n";
import type { List } from "./api";
import { HeadRow, IssueRow, SkeletonRows, type RowActions } from "./IssueRow";
import { chipOf, filterRows, labelsIn, MINE_QUERY, OPEN_QUERY, pageCount, pageOf, pagerPages, PER_PAGE } from "./query";

export default function IssueTable({ query, onQuery, list, loading, repo, isLinked, busy, act, picker }: {
  query: string;
  onQuery: (q: string) => void;
  list: List | null;
  loading: boolean;
  repo: string;
  isLinked: (number: number) => boolean;
  busy: number | null; // the issue a write is under way on
  act: RowActions;
  picker: ReactNode;
}) {
  const [draft, setDraft] = useState(query);
  const [page, setPage] = useState(0);
  const now = useNow(30_000);
  useEffect(() => {
    setDraft(query);
    setPage(0);
  }, [query, repo]);
  const items = list?.items ?? [];
  const pages = pageCount(items.length);
  const at = Math.min(page, pages - 1);
  const shown = pageOf(items, at);
  const chip = chipOf(query);
  const filters: MenuItem[] = filterRows(query, labelsIn(items)).map((row) =>
    "heading" in row ? { heading: row.heading } : "sep" in row ? { sep: true } : { label: row.label, hint: row.hint, checked: row.checked, onSelect: () => onQuery(row.query) },
  );
  return (
    <div className="issue-list">
      <div className="issue-bar">
        {picker}
        <div className="issue-chips" role="group" aria-label={t("Show")}>
          <button type="button" className={`btn sm${chip === "open" ? " on" : ""}`} aria-pressed={chip === "open"} onClick={() => onQuery(OPEN_QUERY)}>
            {tc("issues", "Open")}
          </button>
          <button type="button" className={`btn sm${chip === "mine" ? " on" : ""}`} aria-pressed={chip === "mine"} onClick={() => onQuery(MINE_QUERY)}>
            {t("Assigned to me")}
          </button>
        </div>
      </div>
      <div className="issue-bar">
        <Menu
          label={t("Filters")}
          align="start"
          items={filters}
          trigger={
            <button type="button" className="btn issue-filters">
              <Icon name="filter" size={14} /> {t("Filters")}
            </button>
          }
        />
        <form
          className="issue-search"
          role="search"
          onSubmit={(e) => {
            e.preventDefault();
            onQuery(draft);
          }}
        >
          <Icon name="search" size={14} />
          <input value={draft} spellCheck={false} aria-label={t("Search the issues")} placeholder={OPEN_QUERY} onChange={(e) => setDraft(e.target.value)} />
          {query !== OPEN_QUERY && (
            <Tip text={t("Back to the open issues")}>
              <button type="button" className="icon-btn sm" aria-label={t("Back to the open issues")} onClick={() => onQuery(OPEN_QUERY)}>
                <Icon name="x" size={12} />
              </button>
            </Tip>
          )}
        </form>
      </div>
      <div className="issue-table" role="table" aria-label={t("Issues")} aria-busy={loading}>
        <HeadRow />
        <div className="issue-rows" role="rowgroup">
          {loading && !list ? (
            <SkeletonRows />
          ) : shown.length === 0 ? (
            <p className="issue-empty">{t("No issue matches {query}.", { query })}</p>
          ) : (
            shown.map((item) => <IssueRow key={item.number} item={item} repo={repo} me={list?.me ?? ""} linked={isLinked(item.number)} busy={busy === item.number} act={act} now={now} />)
          )}
        </div>
      </div>
      <div className="issue-foot">
        <span className="issue-count">
          {list ? (list.more ? t("The newest {n} found", { n: items.length }) : tn(items.length, "{n} issue", "{n} issues")) : "…"}
          {list && ` · ${t("read {ago}", { ago: agoWords(list.at, now) })}`}
        </span>
        {pages > 1 && (
          <nav className="issue-pager" aria-label={t("Pages")}>
            <button type="button" className="btn ghost sm" disabled={at === 0} onClick={() => setPage(at - 1)}>
              <Icon name="chevron-left" size={12} /> {t("Previous")}
            </button>
            {pagerPages(at, pages).map((k) => (
              <button key={k} type="button" className={`btn ghost sm issue-page${k === at ? " on" : ""}`} aria-current={k === at ? "page" : undefined} onClick={() => setPage(k)}>
                {k + 1}
              </button>
            ))}
            <button type="button" className="btn ghost sm" disabled={at >= pages - 1} onClick={() => setPage(at + 1)}>
              {t("Next")} <Icon name="chevron-right" size={12} />
            </button>
          </nav>
        )}
        <span className="issue-per-page">{t("{n} a page", { n: PER_PAGE })}</span>
      </div>
    </div>
  );
}
