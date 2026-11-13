// The commit picked, under the graph: what the history already knows shows at once — the hash, the parents, the
// author, the date and its age — then the message's body, the committer and the files with their lines as git answers;
// a file opens its patch.

import { useEffect, useState } from "react";
import { useQuery } from "@/lib/query";
import { Icon, Tip } from "@/ui";
import { readCommit, readPatch, type Commit, type CommitDetail, type GitFile } from "./api";
import { CommitDiff } from "./CommitDiff";
import { dateWords, filesWords, stampWords } from "./words";
import { t, tc } from "@/lib/i18n";

const STATUS_TONE: Record<string, string> = { A: "add", D: "del", R: "ren", C: "ren", M: "mod", T: "mod" };

export function CommitDetails({ machineId, dir, commit, now, onClose }: { machineId: string; dir: string; commit: Commit; now: number; onClose: () => void }) {
  const [file, setFile] = useState<GitFile | null>(null);
  useEffect(() => setFile(null), [commit.hash]);
  const detail = useQuery(() => readCommit(machineId, dir, commit.hash), [machineId, dir, commit.hash]);
  const patch = useQuery(() => (file ? readPatch(machineId, dir, commit.hash, file) : Promise.resolve("")), [machineId, dir, commit.hash, file?.path, file?.from], { enabled: !!file });
  const d = detail.data;
  return (
    <section className="git-details" aria-label={t("Commit {hash}", { hash: commit.hash.slice(0, 8) })}>
      <div className="git-details-head">
        {file ? (
          <>
            <button type="button" className="linkish" onClick={() => setFile(null)}>
              <Icon name="arrow-left" size={12} /> {t("Files")}
            </button>
            <span className="git-details-path">{file.from && file.from !== file.path ? `${file.from} → ${file.path}` : file.path}</span>
          </>
        ) : (
          <span className="git-details-title">{t("Commit {hash}", { hash: commit.hash.slice(0, 8) })}</span>
        )}
        <Tip text={t("Close the commit")}>
          <button type="button" className="icon-btn sm" aria-label={t("Close the commit")} onClick={onClose}>
            <Icon name="x" size={14} />
          </button>
        </Tip>
      </div>
      <div className="git-details-body">
        {file ? (
          patch.error ? (
            <p className="git-error-note">{t("Can't show this diff: {reason}", { reason: patch.error })}</p>
          ) : patch.loading ? (
            <Skeleton rows={6} />
          ) : (
            <CommitDiff patch={patch.data ?? ""} path={file.path} binary={file.binary} />
          )
        ) : (
          <>
            <h3 className="git-details-subject">{commit.subject}</h3>
            <dl className="git-meta">
              <dt>{tc("git", "Commit")}</dt>
              <dd className="mono git-meta-hash">{commit.hash}</dd>
              <dt>{t("Parents")}</dt>
              <dd className="mono">{commit.parents.map((p) => p.slice(0, 8)).join(", ") || t("none")}</dd>
              <dt>{t("Author")}</dt>
              <dd>
                {commit.author} &lt;{commit.email}&gt;
              </dd>
              <dt>{t("Date")}</dt>
              <dd>
                {stampWords(commit.time)} · {dateWords(commit.time, now)}
              </dd>
              {d && d.committer && d.committer !== commit.author && (
                <>
                  <dt>{t("Committer")}</dt>
                  <dd>{d.committer}</dd>
                </>
              )}
            </dl>
            {detail.error ? <p className="git-error-note">{detail.error}</p> : !d ? <Skeleton rows={3} /> : <Body detail={d} onOpen={setFile} />}
          </>
        )}
      </div>
    </section>
  );
}

function Body({ detail, onOpen }: { detail: CommitDetail; onOpen: (file: GitFile) => void }) {
  const lines = detail.message.split("\n").slice(1).join("\n").trim();
  const add = detail.files.reduce((n, f) => n + f.add, 0);
  const del = detail.files.reduce((n, f) => n + f.del, 0);
  return (
    <>
      {lines && <p className="git-message">{lines}</p>}
      <div className="git-files-head">
        <span>{filesWords(detail.files.length + detail.more)}</span>
        {add > 0 && <span className="git-add">+{add}</span>}
        {del > 0 && <span className="git-del">−{del}</span>}
      </div>
      <ul className="git-files">
        {detail.files.map((file) => {
          const slash = file.path.lastIndexOf("/");
          return (
            <li key={file.path}>
              <button type="button" className="git-file" onClick={() => onOpen(file)}>
                <span className={`git-file-status is-${STATUS_TONE[file.status] ?? "mod"}`}>{file.status}</span>
                <span className="git-file-path">
                  <span className="git-file-dir">{slash >= 0 ? file.path.slice(0, slash + 1) : ""}</span>
                  {file.path.slice(slash + 1)}
                </span>
                {file.add > 0 && <span className="git-add">+{file.add}</span>}
                {file.del > 0 && <span className="git-del">−{file.del}</span>}
              </button>
            </li>
          );
        })}
      </ul>
      {detail.more > 0 && <p className="git-empty-note">{t("and {n} more", { n: detail.more })}</p>}
    </>
  );
}

// Skeleton is a loading state shaped like what comes: a few rows' bars
function Skeleton({ rows }: { rows: number }) {
  return (
    <div className="git-skeleton" aria-label={t("Loading…")}>
      {Array.from({ length: rows }, (_, i) => (
        <span key={i} style={{ width: `${60 + ((i * 17) % 35)}%` }} />
      ))}
    </div>
  );
}
