// The viewer's right column, as GitHub's: who is assigned (and Assign yourself), the labels, the milestone, the pull
// requests that close it, everyone who took part.

import { openUrl } from "@/lib/host";
import { Icon } from "@/ui";
import { t } from "@/lib/i18n";
import type { Detail } from "./api";
import { Face, LabelChip, Who } from "./pieces";
import { assigned, refWords } from "./words";

const PULL_TONE: Record<string, string> = { OPEN: "open", MERGED: "done", CLOSED: "skipped" };

export default function IssueSide({ issue, repo, busy, onAssign }: { issue: Detail; repo: string; busy: boolean; onAssign: (me: boolean) => void }) {
  const mine = assigned(issue, issue.me);
  return (
    <aside className="issue-side" aria-label={t("About the issue")}>
      <section className="issue-side-part">
        <h4>{t("Assignees")}</h4>
        {issue.assignees.length > 0 ? (
          <ul className="issue-side-people">
            {issue.assignees.map((a) => (
              <li key={a.login}>
                <Who person={a} />
              </li>
            ))}
          </ul>
        ) : (
          <p className="issue-side-none">{t("No one")}</p>
        )}
        {issue.me && (
          <button type="button" className="linkish" disabled={busy} onClick={() => onAssign(!mine)}>
            {mine ? t("Unassign me") : t("Assign yourself")}
          </button>
        )}
      </section>
      <section className="issue-side-part">
        <h4>{t("Labels")}</h4>
        {issue.labels.length > 0 ? (
          <div className="issue-labels is-wrap">
            {issue.labels.map((l) => (
              <LabelChip key={l.name} label={l} />
            ))}
          </div>
        ) : (
          <p className="issue-side-none">{t("No labels")}</p>
        )}
      </section>
      <section className="issue-side-part">
        <h4>{t("Milestone")}</h4>
        {issue.milestone ? (
          <button type="button" className="linkish issue-side-line" disabled={!issue.milestone.url} onClick={() => issue.milestone?.url && openUrl(issue.milestone.url)}>
            <Icon name="milestone" size={14} /> {issue.milestone.title}
          </button>
        ) : (
          <p className="issue-side-none">{t("No milestone")}</p>
        )}
      </section>
      <section className="issue-side-part">
        <h4>{t("Linked pull requests")}</h4>
        {issue.pulls.length > 0 ? (
          <ul className="issue-side-pulls">
            {issue.pulls.map((p) => (
              <li key={p.url}>
                <button type="button" className="linkish issue-side-line" onClick={() => openUrl(p.url)}>
                  <Icon name="git-pull-request" size={14} className={`is-${PULL_TONE[p.state] ?? "open"}`} />
                  <span className="issue-side-pull">
                    <b>{refWords(repo, p.repo, p.number)}</b> {p.title}
                    {p.draft && <span className="issue-badge">{t("draft")}</span>}
                  </span>
                </button>
              </li>
            ))}
          </ul>
        ) : (
          <p className="issue-side-none">{t("No pull request closes it yet")}</p>
        )}
      </section>
      <section className="issue-side-part">
        <h4>{t("Participants")}</h4>
        <div className="issue-side-faces">
          {issue.participants.slice(0, 18).map((p) => (
            <span key={p.login} className="issue-side-face" title={p.login}>
              <Face person={p} size={24} />
            </span>
          ))}
        </div>
      </section>
    </aside>
  );
}
