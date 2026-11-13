// The lens's head, after git-graph's: where HEAD stands, All branches | Current, the remote branches' box, how many
// commits are drawn, a spinner's place kept so nothing moves while a read is out, and a reread.

import { Icon, Tip } from "@/ui";
import type { Scope } from "./api";
import { whereWords } from "./words";
import { t, tc } from "@/lib/i18n";

export function GitHeader({ branch, head, scope, onScope, count, fetching, onRefresh }: {
  branch?: string;
  head?: string;
  scope: Scope;
  onScope: (scope: Scope) => void;
  count: string;
  fetching: boolean;
  onRefresh: () => void;
}) {
  const where = whereWords(branch, head);
  return (
    <div className="git-head">
      {where && (
        <span className="git-where" title={where}>
          <Icon name={branch ? "git-branch" : "git-commit"} size={12} />
          <span>{where}</span>
        </span>
      )}
      <div className="fv-tabs git-scope" role="group" aria-label={t("Which commits")}>
        <button type="button" className={scope.all ? "on" : ""} aria-pressed={scope.all} onClick={() => onScope({ ...scope, all: true })}>
          {t("All branches")}
        </button>
        <button type="button" className={scope.all ? "" : "on"} aria-pressed={!scope.all} onClick={() => onScope({ ...scope, all: false })}>
          {tc("git", "Current")}
        </button>
      </div>
      <label className="git-remotes">
        <input type="checkbox" checked={scope.remotes} onChange={(e) => onScope({ ...scope, remotes: e.target.checked })} />
        {t("Remotes")}
      </label>
      <span className="git-count">{count}</span>
      <span className="git-spin" aria-hidden="true">
        {fetching && <Icon name="refresh-cw" size={12} className="spin" />}
      </span>
      <Tip text={t("Read the history again")}>
        <button type="button" className="icon-btn sm" aria-label={t("Read the history again")} onClick={onRefresh}>
          <Icon name="refresh-cw" size={14} />
        </button>
      </Tip>
    </div>
  );
}
