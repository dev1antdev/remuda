// One row of the issues table, after Orca's: the number in a pill, the title with its author and labels under it, the
// assignees with a menu to assign or unassign yourself, the state with its menu (Close as completed / not planned, or
// Reopen), how long ago it moved, and Start — or Open when a workspace already works on it — beside a ⋯ of copies.

import { memo } from "react";
import { copyText, openUrl } from "@/lib/host";
import { actions } from "@/app/store";
import { agoWords } from "@/lib/time";
import { Icon, Menu, Tip, type MenuItem } from "@/ui";
import { t } from "@/lib/i18n";
import type { Item } from "./api";
import { Face, Labels, StatePill } from "./pieces";
import { assigned } from "./words";

export type RowActions = {
  view: (item: Item) => void;
  start: (item: Item) => void;
  open: (item: Item) => void;
  assign: (item: Item, me: boolean) => void;
  close: (item: Item, reason: "completed" | "not_planned") => void;
  reopen: (item: Item) => void;
  task: (item: Item) => void;
};

const copy = (text: string) => void copyText(text).then((ok) => ok && actions.notify(t("Copied"), { brief: true }));

export const IssueRow = memo(function IssueRow({ item, repo, me, linked, busy, act, now }: { item: Item; repo: string; me: string; linked: boolean; busy: boolean; act: RowActions; now: number }) {
  const mine = assigned(item, me);
  const people: MenuItem[] = [
    { heading: t("Assignees") },
    ...(item.assignees.length ? item.assignees.map((a) => ({ label: a.login, checked: true, disabled: true, onSelect: () => {} })) : [{ label: t("No one"), disabled: true, onSelect: () => {} }]),
  ];
  if (me) people.push({ sep: true }, mine ? { label: t("Unassign me"), icon: "x", onSelect: () => act.assign(item, false) } : { label: t("Assign yourself"), icon: "user", onSelect: () => act.assign(item, true) });
  const states: MenuItem[] =
    item.state === "open"
      ? [
          { heading: t("Status") },
          { label: t("Close as completed"), icon: "circle-check", hint: t("Done, fixed, resolved"), onSelect: () => act.close(item, "completed") },
          { label: t("Close as not planned"), icon: "circle-slash", hint: t("Won't fix, duplicate, stale"), onSelect: () => act.close(item, "not_planned") },
        ]
      : [{ heading: t("Status") }, { label: t("Reopen"), icon: "circle-dot", onSelect: () => act.reopen(item) }];
  const more: MenuItem[] = [
    { label: t("Copy link"), icon: "link", hint: item.url, onSelect: () => copy(item.url) },
    { label: t("Copy reference"), icon: "copy", hint: `${repo}#${item.number}`, onSelect: () => copy(`${repo}#${item.number}`) },
    { label: t("Copy number and title"), icon: "copy", hint: `#${item.number} ${item.title}`, onSelect: () => copy(`#${item.number} ${item.title}`) },
    { sep: true },
    { label: t("New task from this issue…"), icon: "list", hint: t("into your Tasks, the issue named in it"), onSelect: () => act.task(item) },
    { label: t("Open on GitHub"), icon: "external-link", onSelect: () => openUrl(item.url) },
  ];
  return (
    <div className={`issue-row${busy ? " is-busy" : ""}`} role="row" data-issue={item.number} onClick={() => act.view(item)}>
      <span className="issue-cell issue-id" role="cell">
        <span className="issue-id-pill">#{item.number}</span>
      </span>
      <span className="issue-cell issue-main" role="cell">
        <button
          type="button"
          className="linkish issue-title"
          onClick={(e) => {
            e.stopPropagation();
            act.view(item);
          }}
        >
          {item.title}
        </button>
        <span className="issue-context">
          <span className="issue-author">{item.author.login}</span>
          {item.author.bot && <span className="issue-badge">{t("bot")}</span>}
          <span className="issue-when issue-when-inline">{agoWords(item.updated, now)}</span>
          <Labels labels={item.labels} />
        </span>
      </span>
      <span className="issue-cell issue-people" role="cell" onClick={(e) => e.stopPropagation()}>
        <Menu
          label={t("Assignees")}
          tip={item.assignees.length ? item.assignees.map((a) => a.login).join(", ") : t("No one is assigned")}
          items={people}
          trigger={
            <button type="button" className="btn ghost sm issue-faces" aria-label={t("Assignees")}>
              {item.assignees.length === 0 ? <span className="issue-nobody">–</span> : item.assignees.slice(0, 3).map((a) => <Face key={a.login} person={a} />)}
              <Icon name="chevron-down" size={12} />
            </button>
          }
        />
      </span>
      <span className="issue-cell issue-status" role="cell" onClick={(e) => e.stopPropagation()}>
        <Menu
          label={t("Status")}
          items={states}
          trigger={
            <button type="button" className="btn ghost sm issue-state-btn" disabled={busy}>
              <StatePill issue={item} chevron />
            </button>
          }
        />
      </span>
      <span className="issue-cell issue-updated" role="cell">
        {agoWords(item.updated, now)}
      </span>
      <span className="issue-cell issue-actions" role="cell" onClick={(e) => e.stopPropagation()}>
        {linked ? (
          <button type="button" className="btn sm primary issue-go" onClick={() => act.open(item)}>
            {t("Open")} <Icon name="arrow-right" size={12} />
          </button>
        ) : (
          <button type="button" className="btn sm issue-go" onClick={() => act.start(item)}>
            {t("Start")} <Icon name="arrow-right" size={12} />
          </button>
        )}
        <Menu
          label={t("More")}
          tip={t("Copy, a task, GitHub")}
          items={more}
          trigger={
            <button type="button" className="icon-btn sm" aria-label={t("More")}>
              <Icon name="more" size={14} />
            </button>
          }
        />
      </span>
    </div>
  );
});

// SkeletonRows stand where the rows will be while the first list is read, shaped like them
export function SkeletonRows({ n = 4 }: { n?: number }) {
  return (
    <>
      {Array.from({ length: n }, (_, i) => (
        <div key={i} className="issue-row is-skeleton" aria-hidden="true">
          <span className="issue-cell issue-id">
            <span className="issue-bone is-pill" />
          </span>
          <span className="issue-cell issue-main">
            <span className="issue-bone is-title" />
            <span className="issue-bone is-line" />
          </span>
        </div>
      ))}
    </>
  );
}

export function HeadRow() {
  return (
    <div className="issue-row is-head" role="row">
      <span className="issue-cell issue-id" role="columnheader">
        {t("ID")}
      </span>
      <span className="issue-cell issue-main" role="columnheader">
        {t("Title")}
      </span>
      <span className="issue-cell issue-people" role="columnheader">
        {t("Assignees")}
      </span>
      <span className="issue-cell issue-status" role="columnheader">
        {t("Status")}
      </span>
      <span className="issue-cell issue-updated" role="columnheader">
        {t("Updated")}
      </span>
      <span className="issue-cell issue-actions" role="columnheader">
        <Tip text={t("Start makes a worktree for the issue; Open goes to the workspace already on it")}>
          <span className="issue-head-hint">
            <Icon name="info" size={12} />
          </span>
        </Tip>
      </span>
    </div>
  );
}
