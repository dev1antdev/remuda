// What is committed and not on the remote yet, as better-changes' Committed group has it: each commit its subject, who
// and when, its short hash; a click opens its files, and each file its diff in that commit. Read as of the last fetch.

import { useState, type ReactNode } from "react";
import { t } from "@/lib/i18n";
import { useQuery } from "@/lib/query";
import { ago } from "@/lib/time";
import { Icon, Notice } from "@/ui";
import { fetchCommit, type OutCommit, type Outgoing, type ReviewFile } from "./api";
import type { RowLook } from "./Diff";
import { outgoingTitle } from "./scope";

export function OutgoingCommits({ machineId, dir, outgoing, fileRow }: {
  machineId: string;
  dir: string;
  outgoing: Outgoing;
  fileRow: (file: ReviewFile, look: RowLook) => ReactNode;
}) {
  const [open, setOpen] = useState<string | null>(null);
  if (outgoing.total === 0) return null;
  return (
    <section className="review-group is-outgoing" aria-label={outgoingTitle(outgoing)}>
      <div className="review-group-head">
        <span className="review-group-title">{outgoingTitle(outgoing)}</span>
        <span className="badge quiet">{outgoing.total}</span>
      </div>
      {outgoing.commits.map((commit) => (
        <div key={commit.sha} className={`review-outgoing${open === commit.sha ? " open" : ""}`}>
          <button type="button" className="review-outgoing-head" aria-expanded={open === commit.sha} onClick={() => setOpen(open === commit.sha ? null : commit.sha)}>
            <span className={`chevron${open === commit.sha ? " open" : ""}`}>
              <Icon name="chevron-right" size={12} />
            </span>
            <span className="review-outgoing-text">
              <span className="review-outgoing-subject">{commit.subject}</span>
              <span className="review-outgoing-by">
                {commit.author} · {ago(commit.when * 1000)}
              </span>
            </span>
            <code className="review-outgoing-sha">{commit.short}</code>
          </button>
          {open === commit.sha && <CommitFiles machineId={machineId} dir={dir} commit={commit} fileRow={fileRow} />}
        </div>
      ))}
      {outgoing.total > outgoing.commits.length && <p className="review-group-more">{t("…and {n} older ones.", { n: outgoing.total - outgoing.commits.length })}</p>}
    </section>
  );
}

// CommitFiles is one commit opened: its message's body and the files it changed, each opening its diff in the commit
function CommitFiles({ machineId, dir, commit, fileRow }: { machineId: string; dir: string; commit: OutCommit; fileRow: (file: ReviewFile, look: RowLook) => ReactNode }) {
  const { data: view, error } = useQuery(() => fetchCommit(machineId, dir, commit.sha), [machineId, dir, commit.sha]);
  if (error) return <Notice tone="error">{error}</Notice>;
  if (!view) return <div className="empty">{t("Loading…")}</div>;
  return (
    <div className="review-outgoing-body">
      {view.body && <p className="review-outgoing-message">{view.body}</p>}
      {view.files.length === 0 && <div className="empty">{t("No file of this folder changed in this commit.")}</div>}
      {view.files.map((changed) =>
        fileRow(
          { path: changed.path, added: changed.added, removed: changed.removed, fresh: false },
          { key: `commit-${commit.sha}:${changed.path}`, stat: { added: changed.added, removed: changed.removed, binary: changed.binary }, letter: changed.status, from: { commit: commit.sha } },
        ),
      )}
    </div>
  );
}
