// The two lists: the branch's pull requests, and the repository's — searched as typed (GitHub's own qualifiers such as
// author: and label: passed through), All / Created / Involved, Open / Draft / Merged / Closed / All, a page at a time.

import { useEffect, useState } from "react";
import type { Query } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { ago } from "@/lib/time";
import { t } from "@/lib/i18n";
import { openUrl } from "@/lib/host";
import { Icon, Notice, Select, Tip } from "@/ui";
import { fetchSearch, type BranchAnswer, type Card, type Found, type Remote } from "./api";
import { cardChecks, kindIcon, kindTone, kindWords, prLook, searchText } from "./model";

export type RepoAsk = { text: string; scope: string; state: string };

// PRRow is a list's row: the state's icon, the title, the checks' icon, and under it the author and age, the number
export function PRRow({ card, where, onOpen }: { card: Card; where: string; onOpen: () => void }) {
  const look = prLook(card);
  const checks = cardChecks(card);
  return (
    <button type="button" className="pr-row" onClick={onOpen}>
      <span className={`pr-state-icon is-${look.tone}`} aria-label={look.label}>
        <Icon name={look.icon} size={14} />
      </span>
      <span className="pr-row-title">{card.title}</span>
      <span className="pr-row-checks">{checks && <Icon name={kindIcon(checks)} size={14} className={`is-${kindTone(checks)}`} aria-label={t("Checks: {state}", { state: kindWords(checks) })} />}</span>
      <span className="pr-row-meta">
        {card.author} · {ago(card.updated_at)} · {where}
      </span>
      <span className="pr-row-number">#{card.number}</span>
    </button>
  );
}

// Rows is the loading shape of a list: rows of grey, no spinner in the middle of nothing
function Rows() {
  return (
    <div className="pr-list" aria-busy="true">
      {[0, 1, 2].map((i) => (
        <div key={i} className="pr-row is-ghost" />
      ))}
    </div>
  );
}

export function BranchList({ query, branch, onOpen }: { query: Query<BranchAnswer>; branch: string; onOpen: (card: Card) => void }) {
  if (query.error) return <Notice tone="error">{query.error}</Notice>;
  if (!query.data) return <Rows />;
  const items = query.data.items ?? [];
  if (!items.length) return <p className="pr-empty">{branch ? t("No pull request comes from {branch}.", { branch }) : t("Check out a branch to see its pull requests.")}</p>;
  return (
    <div className="pr-list">
      {items.map((card) => (
        <PRRow key={card.id} card={card} where={card.repository} onOpen={() => onOpen(card)} />
      ))}
    </div>
  );
}

const SCOPES = () => [
  { value: "all", label: t("All pull requests") },
  { value: "mine", label: t("Created by me") },
  { value: "involved", label: t("Involving me") },
];
const STATES = () => [
  { value: "open", label: t("Open") },
  { value: "draft", label: t("Draft") },
  { value: "merged", label: t("Merged") },
  { value: "closed", label: t("Closed") },
  { value: "all", label: t("All") },
];

export function RepoList({ query, repo, remotes, ask, onAsk, onRemote, machineId, onOpen }: {
  query: Query<Found>;
  repo: string;
  remotes: Remote[];
  ask: RepoAsk;
  onAsk: (ask: RepoAsk) => void;
  onRemote: (repo: string) => void;
  machineId: string;
  onOpen: (card: Card) => void;
}) {
  const [more, setMore] = useState<{ items: Card[]; cursors: Record<string, string> | null }>({ items: [], cursors: null });
  const [loadingMore, setLoadingMore] = useState(false);
  const [moreError, setMoreError] = useState("");
  const first = query.data;
  useEffect(() => setMore({ items: [], cursors: null }), [first]);
  const cursors = more.cursors ?? first?.cursors ?? {};
  const seen = new Set<string>();
  const items = [...(first?.items ?? []), ...more.items].filter((c) => !seen.has(c.id) && seen.add(c.id));
  const loadMore = async () => {
    setLoadingMore(true);
    setMoreError("");
    try {
      const page = await fetchSearch(machineId, repo, { text: searchText(ask.text), scope: ask.scope, state: ask.state, cursors });
      setMore((m) => ({ items: [...m.items, ...page.items], cursors: page.cursors }));
    } catch (e) {
      setMoreError(errMsg(e));
    } finally {
      setLoadingMore(false);
    }
  };
  const left = first?.total !== undefined ? first.total - items.length : null;
  const github = remotes.filter((r) => r.repo);
  return (
    <>
      <div className="pr-filters">
        <input type="search" className="sm" placeholder={t("Search · #482, words, author:login, label:bug")} value={ask.text} onChange={(e) => onAsk({ ...ask, text: e.target.value })} aria-label={t("Search the pull requests")} />
        <div className="pr-filter-row">
          <Select value={ask.scope} options={SCOPES()} onChange={(scope) => onAsk({ ...ask, scope })} />
          <Select value={ask.state} options={STATES()} onChange={(state) => onAsk({ ...ask, state })} />
          {github.length > 1 && <Select value={repo} options={github.map((r) => ({ value: r.repo!, label: r.name, hint: r.repo }))} onChange={onRemote} />}
          {repo && (
            <Tip text={t("Open {repo} on GitHub", { repo })}>
              <button type="button" className="icon-btn" aria-label={t("Open {repo} on GitHub", { repo })} onClick={() => openUrl(`https://github.com/${repo}/pulls`)}>
                <Icon name="external-link" size={16} />
              </button>
            </Tip>
          )}
        </div>
      </div>
      {!repo ? (
        <p className="pr-empty">{t("The checkout has no GitHub remote.")}</p>
      ) : query.error ? (
        <Notice tone="error">{query.error}</Notice>
      ) : !first ? (
        <Rows />
      ) : items.length === 0 ? (
        <p className="pr-empty">{t("No pull request matches.")}</p>
      ) : (
        <div className="pr-list">
          {items.map((card) => (
            <PRRow key={card.id} card={card} where={card.head} onOpen={() => onOpen(card)} />
          ))}
          {Object.keys(cursors).length > 0 && (
            <button type="button" className="linkish pr-more" disabled={loadingMore} onClick={() => void loadMore()}>
              {loadingMore ? t("Loading…") : left !== null && left > 0 ? t("+ {n} more", { n: Math.min(left, 25) }) : t("+ more")}
            </button>
          )}
          {moreError && <Notice tone="error">{moreError}</Notice>}
        </div>
      )}
    </>
  );
}
