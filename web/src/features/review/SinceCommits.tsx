// The commits made since the last look, above the files: each a short hash, its subject and when, the ones of work
// already reviewed dimmed.

import { t } from "@/lib/i18n";
import { ago } from "@/lib/time";
import type { ReviewCommit } from "./api";

export function SinceCommits({ commits, total }: { commits: ReviewCommit[]; total?: number }) {
  if (commits.length === 0) return null;
  return (
    <section className="review-group is-since" aria-label={t("Committed since you looked")}>
      <div className="review-group-head">
        <span className="review-group-title">{t("Committed since you looked")}</span>
        <span className="badge quiet">{total ?? commits.length}</span>
      </div>
      <div className="review-commits">
        {commits.map((commit) => (
          <div key={commit.sha} className={`review-commit${commit.seen ? " seen" : ""}`} title={commit.seen ? t("A commit of work you already reviewed") : undefined}>
            <code>{commit.sha}</code>
            <span className="review-commit-subject">{commit.subject}</span>
            {commit.seen && <span className="review-tag">{t("reviewed")}</span>}
            <span className="review-commit-when">{ago(commit.when * 1000)}</span>
          </div>
        ))}
      </div>
    </section>
  );
}
