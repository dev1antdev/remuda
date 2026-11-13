// One issue inside the panel, after Orca's viewer: Back, the title with its number and a filled state pill, Start or
// Open its workspace; the timeline and the comment box on the left, the right column beside them. A link to another
// issue opens here, Back going to the one before; a pull request opens on GitHub.

import { useRef, useState } from "react";
import { actions } from "@/app/store";
import { openUrl } from "@/lib/host";
import { useQuery } from "@/lib/query";
import { ago } from "@/lib/time";
import { errMsg } from "@/lib/text";
import { Icon, Tip } from "@/ui";
import { t, tn } from "@/lib/i18n";
import { assignIssue, closeIssue, commentIssue, fetchIssue, reopenIssue, troubleOf, type Detail } from "./api";
import Composer from "./Composer";
import IssueSide from "./IssueSide";
import { StatePill, TroubleNote, Who } from "./pieces";
import Timeline from "./Timeline";

export type Viewed = { repo: string; number: number };

export default function IssueViewer({ machineId, folder, at, back, onBack, onGo, onStart, onOpen, linked, onChanged }: {
  machineId: string;
  folder: string; // the repository's checkout on the machine: a path in the issue opens there
  at: Viewed;
  back: string; // what Back goes to: the list, or the issue before
  onBack: () => void;
  onGo: (repo: string, number: number) => void;
  onStart: (issue: Detail) => void;
  onOpen: (issue: Detail) => void;
  linked: (repo: string, number: number) => boolean;
  onChanged: () => void; // a write went through: the list reads again
}) {
  const fresh = useRef(false);
  const query = useQuery(
    () => {
      const ask = fresh.current;
      fresh.current = false;
      return fetchIssue(machineId, at.repo, at.number, ask);
    },
    [machineId, at.repo, at.number],
    { every: 60_000 },
  );
  const [busy, setBusy] = useState<string | null>(null);
  const trouble = troubleOf(query.data ?? undefined);
  const issue = query.data && !trouble ? (query.data as Detail) : null;
  const again = () => {
    fresh.current = true;
    query.reload();
  };
  const write = async (doing: string, run: () => Promise<unknown>, after?: () => void): Promise<boolean> => {
    setBusy(doing);
    try {
      await run();
      after?.();
      onChanged();
      return true;
    } catch (e) {
      actions.notify(errMsg(e), { error: true });
      return false;
    } finally {
      setBusy(null);
      again();
    }
  };
  const openFile = (target: { path: string; line?: number }) => actions.openFile(machineId, "", target, folder);
  return (
    <div className="issue-viewer">
      <div className="issue-viewer-bar">
        <button type="button" className="btn ghost sm" onClick={onBack}>
          <Icon name="arrow-left" size={14} /> {back}
        </button>
        <span className="issue-grow" />
        {query.fetching && <span className="issue-reading">{t("Reading…")}</span>}
        {issue && (
          <Tip text={t("Open on GitHub")}>
            <button type="button" className="icon-btn" aria-label={t("Open on GitHub")} onClick={() => openUrl(issue.url)}>
              <Icon name="external-link" size={14} />
            </button>
          </Tip>
        )}
      </div>
      {query.error && <TroubleNote trouble={{ kind: "failed", text: query.error }} onRetry={again} />}
      {trouble && <TroubleNote trouble={trouble} onRetry={again} />}
      {!issue && !trouble && !query.error && (
        <div className="issue-viewer-loading" aria-busy="true">
          <span className="issue-bone is-heading" />
          <span className="issue-bone is-line" />
          <span className="issue-bone is-card" />
        </div>
      )}
      {issue?.pull && (
        <p className="modal-hint">
          {t("#{n} is a pull request, not an issue.", { n: issue.number })}{" "}
          <button type="button" className="linkish" onClick={() => openUrl(issue.url)}>
            {t("Open it on GitHub")}
          </button>
        </p>
      )}
      {issue && !issue.pull && (
        <>
          <header className="issue-viewer-head">
            <h3 className="issue-viewer-title">
              {issue.title} <span className="issue-dim">#{issue.number}</span>
            </h3>
            {linked(at.repo, issue.number) ? (
              <button type="button" className="btn sm primary" onClick={() => onOpen(issue)}>
                {t("Open workspace")} <Icon name="arrow-right" size={12} />
              </button>
            ) : (
              <button type="button" className="btn sm" onClick={() => onStart(issue)}>
                {t("Start workspace")} <Icon name="arrow-right" size={12} />
              </button>
            )}
          </header>
          <div className="issue-viewer-sub">
            <StatePill issue={issue} filled />
            <Who person={issue.author} />
            <span className="issue-dim">
              {t("opened this {ago}", { ago: ago(issue.created) })} · {tn(issue.comments, "{n} comment", "{n} comments")}
            </span>
          </div>
          <div className="issue-viewer-body">
            <div className="issue-thread">
              <Timeline issue={issue} repo={at.repo} onGo={onGo} onOpenFile={openFile} />
              <Composer
                issue={issue}
                repo={at.repo}
                draftKey={`issues.draft:${machineId}|${at.repo}#${at.number}`}
                busy={busy}
                onOpenFile={openFile}
                onComment={(body) => write(t("Commenting…"), () => commentIssue(machineId, at.repo, at.number, body))}
                onClose={(reason, body) =>
                  write(t("Closing…"), () => closeIssue(machineId, at.repo, at.number, reason, body), () => actions.setIssueClosed({ machineId, repo: at.repo, number: at.number, reason }))
                }
                onReopen={(body) => write(t("Reopening…"), () => reopenIssue(machineId, at.repo, at.number, body))}
              />
            </div>
            <IssueSide issue={issue} repo={at.repo} busy={!!busy} onAssign={(me) => void write(me ? t("Assigning…") : t("Unassigning…"), () => assignIssue(machineId, at.repo, at.number, me))} />
          </div>
        </>
      )}
    </div>
  );
}
