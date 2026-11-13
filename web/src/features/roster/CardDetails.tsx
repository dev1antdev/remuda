// What a workspace's hover card holds, after Orca's: the note, the pull requests with their checks and review, the
// issues its work names or its request closes, the ports its checkout serves — each section only when it has
// something — and how fresh the requests' state is. CardHover.tsx opens it beside the card.

import { actions } from "@/app/store";
import { copyText, openUrl } from "@/lib/host";
import { useNow } from "@/lib/clock";
import { t } from "@/lib/i18n";
import { Icon, Tip } from "@/ui";
import { tipLines } from "./CardBadges";
import { freshnessOf, freshWords, noteDir, type CardIssue, type Details } from "./cardFacts";
import { prHost, prLook, prNumber, prRepo, prTitle, type CardPR } from "./cardPR";
import { checksTone, checksWords, reviewTone, reviewWords } from "./prStatus";
import type { LivePort, Machine, Session } from "./api";

export function CardDetails({ machine, session, name, details, about, editable }: { machine: Machine; session: Session; name: string; details: Details; about: string[]; editable: boolean }) {
  const { note, prs, issues, ports } = details;
  const editNote = () => actions.openDialog({ kind: "note", machineId: machine.id, dir: noteDir(session), name, text: note });
  return (
    <div className="hc">
      {about.length > 0 && (
        <div className="hc-about">
          {about.map((line, i) => (
            <span key={i}>{line}</span>
          ))}
        </div>
      )}
      {note && (
        <section className="hc-section">
          <h3 className="hc-label">{t("Note")}</h3>
          {editable ? (
            <Tip text={t("Edit the note")}>
              <button type="button" className="linkish hc-note" onClick={editNote}>
                {note}
              </button>
            </Tip>
          ) : (
            <p className="hc-note">{note}</p>
          )}
        </section>
      )}
      {prs.length > 0 && (
        <section className="hc-section">
          <h3 className="hc-label">{prs.length > 1 ? t("Pull requests · {n}", { n: prs.length }) : t("Pull request")}</h3>
          {prs.map((p) => (
            <PRItem key={p.pr.url} item={p} />
          ))}
        </section>
      )}
      {issues.length > 0 && (
        <section className="hc-section">
          <h3 className="hc-label">{issues.length > 1 ? t("Issues · {n}", { n: issues.length }) : t("Issue")}</h3>
          {issues.map((issue) => (
            <IssueItem key={issue.url} issue={issue} />
          ))}
        </section>
      )}
      {ports.length > 0 && (
        <section className="hc-section">
          <h3 className="hc-label">{t("Live ports · {n}", { n: ports.length })}</h3>
          {ports.map((port) => (
            <PortRow key={`${port.pid}:${port.port}`} port={port} />
          ))}
        </section>
      )}
      {prs.length > 0 && <Freshness prs={prs} />}
    </div>
  );
}

// PRItem is one request: its state's icon, its title, its number, state and repository, its checks and review, and
// copy and open on its site at the right
function PRItem({ item: { pr } }: { item: CardPR }) {
  const look = prLook(pr);
  const status = pr.status;
  const review = status ? reviewWords(status) : "";
  return (
    <div className="hc-item">
      <span className={`hc-icon tone-${look.tone}`}>
        <Icon name={look.icon} size={14} />
      </span>
      <span className="hc-text">
        <span className="hc-title">{prTitle(pr)}</span>
        <span className="hc-meta">
          <span>{prNumber(pr)}</span>
          <span className={`hc-state tone-${look.tone}`}>{look.label}</span>
          <span className="hc-where">{prRepo(pr.url)}</span>
        </span>
        {status && (
          <span className="hc-chips">
            <span className={`pr-state is-${checksTone(status)}`}>
              <i className="pr-dot" aria-hidden="true" />
              {checksWords(status)}
            </span>
            {review && <span className={`pr-state is-${reviewTone(status)}`}>{review}</span>}
          </span>
        )}
      </span>
      <LinkButtons url={pr.url} host={prHost(pr.url)} />
    </div>
  );
}

// IssueItem is a Jira issue the work names, or a GitHub issue the request closes
function IssueItem({ issue }: { issue: CardIssue }) {
  return (
    <div className="hc-item">
      <span className={`hc-icon${issue.jira ? ` jira-chip tone-${issue.tone}` : " tone-none"}`}>{issue.jira ? <i className="jira-dot" aria-hidden="true" /> : <Icon name="circle-dot" size={14} />}</span>
      <span className="hc-text">
        <span className="hc-title">{issue.title}</span>
        <span className="hc-meta">
          <span>{issue.key}</span>
          {issue.state && <span className="hc-state">{issue.state}</span>}
          <span className="hc-where">{issue.where}</span>
        </span>
      </span>
      <LinkButtons url={issue.url} host={issue.jira ? "Jira" : "GitHub"} />
    </div>
  );
}

// PortRow is a port the checkout serves: a dot, the port, what listens and where
function PortRow({ port }: { port: LivePort }) {
  return (
    <div className="hc-port">
      <i className="hc-port-dot" aria-hidden="true" />
      <span className="hc-port-num">{port.port}</span>
      <span className="hc-port-proc">{port.process}</span>
      <span className="hc-port-addr">{port.address}</span>
    </div>
  );
}

// LinkButtons copy an address and open it on its site
function LinkButtons({ url, host }: { url: string; host: string }) {
  const copy = async () => {
    if (await copyText(url)) actions.notify(t("Link copied"), { brief: true });
  };
  return (
    <span className="hc-links">
      <Tip text={t("Copy link")}>
        <button type="button" className="icon-btn sm" aria-label={t("Copy link")} onClick={() => void copy()}>
          <Icon name="copy" size={14} />
        </button>
      </Tip>
      <Tip text={t("Open on {host}", { host })}>
        <button type="button" className="icon-btn sm" aria-label={t("Open on {host}", { host })} onClick={() => openUrl(url)}>
          <Icon name="external-link" size={14} />
        </button>
      </Tip>
    </span>
  );
}

// Freshness is the card's foot: a read under way, a read that failed with how old the state shown is, or when the
// site last answered
function Freshness({ prs }: { prs: CardPR[] }) {
  const now = useNow(15_000);
  const f = freshnessOf(prs);
  const words = freshWords(f, now);
  if (!words) return null;
  const line = (
    <div className={`hc-foot${f.kind === "failed" ? " is-failed" : ""}`}>
      {f.kind === "reading" && <span className="spinner" aria-hidden="true" />}
      {words}
    </div>
  );
  return f.kind === "failed" && f.reason ? <Tip text={tipLines(f.reason)}>{line}</Tip> : line;
}
