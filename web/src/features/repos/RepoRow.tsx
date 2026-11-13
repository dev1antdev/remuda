// One repository's row, 34 px as Manygit draws it: the name (in the accent when it is the pane on screen's), a dot
// with the uncommitted count, ↑ green and ↓ red, or the word for a branch on no remote, no remote at all, an upstream
// gone; "…" while it is read; on the right the branch and how long ago it was fetched. A click opens its menu.

import { shortAge } from "@/features/roster/workspaces";
import { ago } from "@/lib/time";
import { t } from "@/lib/i18n";
import { Icon, Menu, type MenuItem } from "@/ui";
import type { Repo } from "./api";
import { dirtyWords, syncLabel, syncOf, syncTip } from "./repos";

export function RepoRow({ repo, name, parent, here, busy, fetchFailed, now, menu }: { repo: Repo; name: string; parent: string; here: boolean; busy: boolean; fetchFailed: string; now: number; menu: () => MenuItem[] }) {
  const kind = syncOf(repo, busy, fetchFailed);
  const counted = kind === "ahead" || kind === "behind" || kind === "diverged";
  const label = syncLabel(kind);
  const problem = kind === "failed" ? fetchFailed : kind === "error" ? (repo.error ?? "") : "";
  const branch = repo.detached ? t("detached") : repo.branch;
  const fetched = repo.fetched ? Date.parse(repo.fetched) : 0;
  const tip = (
    <span className="repo-tip">
      <span>{`${parent}/${name}`}</span>
      {problem && <span>{problem}</span>}
      {!busy && syncTip(repo, kind) && <span>{syncTip(repo, kind)}</span>}
      {repo.dirty > 0 && <span>{dirtyWords(repo)}</span>}
      <span>{fetched ? t("Fetched {when}", { when: ago(fetched, now) }) : t("Never fetched here")}</span>
    </span>
  );
  return (
    <Menu
      label={t("Repository {name}", { name })}
      align="start"
      tip={tip}
      items={menu}
      trigger={
        <button type="button" className={`repo-row${here ? " is-here" : ""}${busy ? " is-busy" : ""}`} aria-label={t("Repository {name}", { name })}>
          <span className="repo-name">{name}</span>
          <span className="repo-state">
            {repo.dirty > 0 && (
              <span className="repo-dirty">
                <span className="repo-dot" aria-hidden="true" />
                {repo.dirty}
              </span>
            )}
            {counted && repo.ahead > 0 && <span className="repo-ahead">↑{repo.ahead}</span>}
            {counted && repo.behind > 0 && <span className="repo-behind">↓{repo.behind}</span>}
            {label && <span className={`repo-word is-${kind}`}>{label}</span>}
          </span>
          <span className="repo-fill" />
          {problem ? (
            <span className="repo-problem">{problem.split("\n").pop()}</span>
          ) : (
            branch && (
              <span className="repo-branch">
                <Icon name="git-branch" size={12} />
                <span className="repo-branch-name">{branch}</span>
              </span>
            )
          )}
          <span className="repo-fetched">{fetched ? shortAge(fetched, now) : "—"}</span>
        </button>
      }
    />
  );
}

// RepoRowLoading is a row's shape while the first read is under way: the name and the state still "…"
export function RepoRowLoading() {
  return (
    <div className="repo-row is-loading" aria-hidden="true">
      <span className="repo-name">…</span>
      <span className="repo-fill" />
      <span className="repo-fetched">…</span>
    </div>
  );
}
