// An issue's timeline as GitHub draws it: the issue's own text first, then each comment as a card — its writer, when,
// their badges, the text as Markdown, the reactions — and every event between them as a small round icon and a line:
// labels added and removed in one go, a rename as old struck through and new, a mention by another issue or pull
// request, a commit that names it, a close, a reopen, a milestone. A link to another issue opens inside, with Back.

import type { MouseEvent } from "react";
import type { OpenFile } from "@/features/files/target";
import { openUrl } from "@/lib/host";
import { ago } from "@/lib/time";
import { Icon, Tip } from "@/ui";
import type { IconName } from "@/ui/icons";
import { Markdown } from "@/ui/Markdown";
import { t, tc, tn } from "@/lib/i18n";
import type { Detail, Event, Reaction } from "./api";
import { LabelChip, Who } from "./pieces";
import { issueAddress, linkify } from "./prompt";
import { REACTIONS, refWords, roleBadges, shortSha } from "./words";

type Go = (repo: string, number: number) => void;

// clickLink sends a GitHub issue's link to the viewer itself; any other link opens in a tab as usual
export function clickLink(onGo: Go) {
  return (e: MouseEvent) => {
    const a = (e.target as HTMLElement).closest?.("a[href]");
    if (!a) return;
    const at = issueAddress(a.getAttribute("href") ?? "");
    if (!at || at.pull) return;
    e.preventDefault();
    e.stopPropagation();
    onGo(at.repo, at.number);
  };
}

// Body is an issue's or a comment's Markdown, its #12s linked; a path in it opens in the repository's checkout
export function Body({ text, repo, onOpenFile }: { text: string; repo: string; onOpenFile: OpenFile }) {
  if (!text.trim()) return <p className="issue-nothing">{t("No description provided.")}</p>;
  return <Markdown text={linkify(text, repo)} onOpenFile={onOpenFile} />;
}

function Reactions({ reactions }: { reactions?: Reaction[] }) {
  if (!reactions?.length) return null;
  return (
    <div className="issue-reactions">
      {reactions.map((r) => (
        <span key={r.content} className="issue-reaction">
          <span className="issue-emoji" aria-hidden="true">
            {REACTIONS[r.content] ?? r.content}
          </span>
          {r.count}
        </span>
      ))}
    </div>
  );
}

type CardProps = { who: Event["actor"]; at: string; body: string; badges: string[]; reactions?: Reaction[]; opened?: boolean; repo: string; url?: string; onOpenFile: OpenFile };

function Card({ who, at, body, badges, reactions, opened, repo, url, onOpenFile }: CardProps) {
  return (
    <article className="issue-card">
      <header className="issue-card-head">
        <Who person={who} />
        <span className="issue-when">{opened ? t("opened {ago}", { ago: ago(at) }) : ago(at)}</span>
        <span className="issue-grow" />
        {badges.map((b) => (
          <span key={b} className="issue-badge">
            {b}
          </span>
        ))}
        {url && (
          <Tip text={t("Open on GitHub")}>
            <button type="button" className="icon-btn sm" aria-label={t("Open on GitHub")} onClick={() => openUrl(url)}>
              <Icon name="external-link" size={12} />
            </button>
          </Tip>
        )}
      </header>
      <div className="issue-card-body">
        <Body text={body} repo={repo} onOpenFile={onOpenFile} />
      </div>
      <Reactions reactions={reactions} />
    </article>
  );
}

function eventLook(e: Event): { icon: IconName; tone: string } {
  switch (e.kind) {
    case "closed":
      return e.reason === "not_planned" ? { icon: "circle-slash", tone: "skipped" } : { icon: "circle-check", tone: "done" };
    case "reopened":
      return { icon: "circle-dot", tone: "open" };
    case "labels":
      return { icon: "tag", tone: "" };
    case "assign":
      return { icon: "user", tone: "" };
    case "renamed":
      return { icon: "edit", tone: "" };
    case "milestone":
      return { icon: "milestone", tone: "" };
    case "mention":
      return { icon: "link", tone: "" };
    default:
      return { icon: "git-commit", tone: "" };
  }
}

function EventWords({ e }: { e: Event }) {
  switch (e.kind) {
    case "labels": {
      const added = e.added ?? [];
      const removed = e.removed ?? [];
      return (
        <>
          {added.length > 0 && <span>{t("added")}</span>}
          {added.map((l) => (
            <LabelChip key={`+${l.name}`} label={l} />
          ))}
          {added.length > 0 && removed.length > 0 && <span>{t("and")}</span>}
          {removed.length > 0 && <span>{t("removed")}</span>}
          {removed.map((l) => (
            <LabelChip key={`-${l.name}`} label={l} />
          ))}
          <span>{tn(added.length + removed.length, "label", "labels")}</span>
        </>
      );
    }
    case "assign":
      if (e.who === e.actor.login) return <span>{e.on ? t("self-assigned this") : t("removed their assignment")}</span>;
      return (
        <>
          <span>{e.on ? t("assigned") : tc("issue", "unassigned")}</span> <b>{e.who}</b>
        </>
      );
    case "closed":
      return <span>{e.reason === "not_planned" ? t("closed this as not planned") : t("closed this as completed")}</span>;
    case "reopened":
      return <span>{t("reopened this")}</span>;
    case "renamed":
      return (
        <>
          <span>{t("changed the title")}</span> <s>{e.from}</s> <b>{e.to}</b>
        </>
      );
    case "milestone":
      return (
        <>
          <span>{e.on ? t("added this to the milestone") : t("removed this from the milestone")}</span> <b>{e.title}</b>
        </>
      );
    case "mention":
      return <span>{t("mentioned this")}</span>;
    case "commit":
      return <span>{t("added a commit that references this issue")}</span>;
  }
  return null;
}

function EventLine({ e, repo, onGo }: { e: Event; repo: string; onGo: Go }) {
  const look = eventLook(e);
  const ref = e.ref;
  const refTone = ref?.merged || ref?.state === "closed" ? "done" : "open";
  return (
    <div className="issue-event">
      <span className={`issue-event-icon${look.tone ? ` is-${look.tone}` : ""}`}>
        <Icon name={look.icon} size={12} />
      </span>
      <div className="issue-event-text">
        <span className="issue-event-line">
          <Who person={e.actor} /> <EventWords e={e} /> <span className="issue-when">{ago(e.at)}</span>
        </span>
        {e.kind === "mention" && ref && (
          <button type="button" className="linkish issue-ref" onClick={() => (ref.pull ? openUrl(ref.url) : onGo(ref.repo || repo, ref.number))}>
            <Icon name={ref.pull ? "git-pull-request" : "circle-dot"} size={14} className={`is-${refTone}`} />
            <b>{ref.title}</b> <span className="issue-dim">{refWords(repo, ref.repo, ref.number)}</span>
          </button>
        )}
        {e.kind === "commit" && e.sha && e.url && (
          <button type="button" className="linkish issue-sha" onClick={() => openUrl(e.url!)}>
            {shortSha(e.sha)}
          </button>
        )}
      </div>
    </div>
  );
}

export default function Timeline({ issue, repo, onGo, onOpenFile }: { issue: Detail; repo: string; onGo: Go; onOpenFile: OpenFile }) {
  return (
    <div className="issue-timeline" onClickCapture={clickLink(onGo)}>
      <Card who={issue.author} at={issue.created} body={issue.body} badges={roleBadges(issue.author.login, "", issue.role)} reactions={issue.reactions} opened repo={repo} onOpenFile={onOpenFile} />
      {issue.timeline.map((e, i) => (
        <div key={i} className="issue-step">
          {e.kind === "comment" ? (
            <Card who={e.actor} at={e.at} body={e.body ?? ""} badges={roleBadges(e.actor.login, issue.author.login, e.role)} reactions={e.reactions} repo={repo} url={e.url} onOpenFile={onOpenFile} />
          ) : (
            <EventLine e={e} repo={repo} onGo={onGo} />
          )}
        </div>
      ))}
    </div>
  );
}
