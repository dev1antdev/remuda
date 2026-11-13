// One pull request: its state, title and number with the way out to GitHub, "author · head → base" with the Actions
// menu (a merged one wears a Merged badge in its place), the stats strip — checks, reviews, unresolved threads, the
// merge's own word — each a link to the tab that explains it, the confirm card a chosen action opens, and the tabs.

import { useState, type ReactNode } from "react";
import type { PRAt } from "@/app/store";
import { openUrl } from "@/lib/host";
import { useQuery } from "@/lib/query";
import { t } from "@/lib/i18n";
import { Icon, Menu, Notice, Tip, type MenuItem } from "@/ui";
import { fetchDetail, type Detail } from "./api";
import { actionLabel, actions as actionsOf, checkCounts, kindIcon, kindTone, prLook, stats, type Action, type Section } from "./model";
import { entries } from "./conversation";
import ConfirmCard from "./ConfirmCard";
import Overview from "./Overview";
import Comments from "./Comments";
import Checks from "./Checks";
import Files from "./Files";

export default function PRDetail({ at, repo, number, stamp, onBack, onClose }: { at: PRAt; repo: string; number: number; stamp: number; onBack?: () => void; onClose: () => void }) {
  const detail = useQuery(() => fetchDetail(at.machineId, repo, number), [at.machineId, repo, number, stamp], { every: 60_000, keep: true });
  const [section, setSection] = useState<Section>("overview");
  const [asking, setAsking] = useState<Action | null>(null);
  const [onlyOpen, setOnlyOpen] = useState(false);
  const pr = detail.data;
  const back = onBack && (
    <Tip text={t("Back to the pull requests")}>
      <button type="button" className="icon-btn" aria-label={t("Back to the pull requests")} onClick={onBack}>
        <Icon name="chevron-left" size={16} />
      </button>
    </Tip>
  );
  if (detail.error && !pr)
    return (
      <div className="pr-detail">
        {back}
        <Notice tone="error">{detail.error}</Notice>
      </div>
    );
  if (!pr)
    return (
      <div className="pr-detail" aria-busy="true">
        <div className="pr-about">
          {back}
          <span className="pr-ghost-line" />
        </div>
        <div className="pr-ghost-block" />
      </div>
    );
  const look = prLook(pr);
  const go = (to: Section | undefined, open = false) => {
    if (!to) return;
    if (open) setOnlyOpen(true);
    setSection(to);
  };
  return (
    <div className="pr-detail">
      <div className="pr-about">
        {back}
        <span className={`pr-state-icon is-${look.tone}`} aria-label={look.label}>
          <Icon name={look.icon} size={16} />
        </span>
        <b className="pr-title">
          {pr.title} <span className="pr-number">#{pr.number}</span>
        </b>
        <Tip text={t("Open on GitHub")}>
          <button type="button" className="icon-btn" aria-label={t("Open on GitHub")} onClick={() => openUrl(pr.url)}>
            <Icon name="external-link" size={16} />
          </button>
        </Tip>
      </div>
      <div className="pr-sub">
        <span className="pr-branches" title={`${pr.head} → ${pr.base}`}>
          {pr.author} · {pr.head} → {pr.base}
        </span>
        <ActionsMenu pr={pr} onPick={setAsking} />
      </div>
      {pr.state === "OPEN" && <StatsStrip pr={pr} onGo={go} />}
      {asking && <ConfirmCard machineId={at.machineId} pr={pr} action={asking} onDone={() => {
            setAsking(null);
            detail.reload();
          }} onCancel={() => setAsking(null)} />}
      {detail.error && <Notice tone="warn">{t("Could not read it again: {error}", { error: detail.error })}</Notice>}
      <SectionTabs pr={pr} section={section} onSection={setSection} />
      <div className="pr-section">
        {section === "overview" && <Overview at={at} pr={pr} onChanged={detail.reload} onClose={onClose} />}
        {section === "comments" && <Comments at={at} pr={pr} onlyOpen={onlyOpen} onOnlyOpen={setOnlyOpen} onChanged={detail.reload} />}
        {section === "checks" && <Checks at={at} pr={pr} />}
        {section === "files" && <Files at={at} pr={pr} />}
      </div>
    </div>
  );
}

// ActionsMenu is what can be done to the pull request, the likeliest first, Close and Reopen set apart; a merged one has
// a Merged badge of the same size in its place, so the line keeps its shape
function ActionsMenu({ pr, onPick }: { pr: Detail; onPick: (a: Action) => void }) {
  const can = actionsOf(pr);
  if (pr.state === "MERGED")
    return (
      <span className="pr-merged">
        <Icon name="git-merge" size={12} /> {t("Merged")}
      </span>
    );
  const items: MenuItem[] = [];
  can.forEach((a, i) => {
    if (i > 0 && (a === "close" || a === "reopen")) items.push({ sep: true });
    items.push({ label: actionLabel(a), icon: a === "close" ? "x" : a === "reopen" ? "refresh-cw" : a === "cancel-auto" ? "x" : "git-merge", danger: a === "close", onSelect: () => onPick(a) });
  });
  return (
    <Menu
      label={t("Actions for this pull request")}
      items={items}
      trigger={
        <button type="button" className="btn sm pr-actions">
          {t("Actions")} <Icon name="chevron-down" size={12} />
        </button>
      }
    />
  );
}

const STAT_TONE = { bad: "is-bad", warn: "is-warn", good: "is-good", dim: "is-dim", none: "is-none" } as const;

// StatsStrip is the one line of figures every open pull request shows, each in its tone and a link to where it is explained
function StatsStrip({ pr, onGo }: { pr: Detail; onGo: (to: Section | undefined, open?: boolean) => void }) {
  return (
    <div className="pr-stats">
      {stats(pr).map((s) => (
        <Tip key={s.key} text={s.detail}>
          <button type="button" className="pr-stat" disabled={!s.section} onClick={() => onGo(s.section, s.key === "unresolved" && s.value !== "0")}>
            <span className="pr-stat-label">{s.label}</span> <b className={STAT_TONE[s.tone]}>{s.value}</b>
          </button>
        </Tip>
      ))}
    </div>
  );
}

function SectionTabs({ pr, section, onSection }: { pr: Detail; section: Section; onSection: (s: Section) => void }) {
  const worst = checkCounts(pr.checks).worst;
  const tab = (id: Section, label: string, extra: ReactNode) => (
    <button type="button" role="tab" aria-selected={section === id} className={section === id ? "on" : ""} onClick={() => onSection(id)}>
      {label} {extra}
    </button>
  );
  return (
    <div className="fv-tabs pr-tabs" role="tablist" aria-label={t("What the pull request shows")}>
      {tab("overview", t("Overview"), null)}
      {tab("comments", t("Comments"), <span className="badge quiet">{entries(pr).length}</span>)}
      {tab("checks", t("Checks"), worst ? <Icon name={kindIcon(worst)} size={12} className={`is-${kindTone(worst)}`} /> : <span className="badge quiet">0</span>)}
      {tab("files", t("Files"), <span className="badge quiet">{pr.changed_files}</span>)}
    </div>
  );
}
