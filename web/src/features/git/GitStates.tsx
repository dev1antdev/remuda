// The lens's other faces: the sentences it says when there is no graph to draw, a loading state shaped like the rows,
// and the banner over the graph while a merge, rebase, cherry-pick or revert waits on its conflicts.

import { Icon, Notice } from "@/ui";
import type { IconName } from "@/ui/icons";
import { stoppedWords } from "./words";
import { t, tc } from "@/lib/i18n";

export function GitEmpty({ icon, title, body }: { icon: IconName; title: string; body?: string }) {
  return (
    <div className="git-empty">
      <span className="git-empty-icon">
        <Icon name={icon} size={20} />
      </span>
      <b>{title}</b>
      {body && <p>{body}</p>}
    </div>
  );
}

// GitLoading is a few rows' worth of lanes and bars while the first page is read
export function GitLoading() {
  return (
    <div className="git-loading" aria-label={t("Reading the history…")}>
      {Array.from({ length: 8 }, (_, i) => (
        <div key={i} className="git-loading-row">
          <span className="git-loading-dot" />
          <span className="git-loading-bar" style={{ width: `${35 + ((i * 23) % 45)}%` }} />
        </div>
      ))}
    </div>
  );
}

// GitStopped is the banner while an operation waits on conflicts: Abort, or the Changes lens to resolve them
export function GitStopped({ operation, conflicts, readOnly, onAbort, onChanges }: { operation: string; conflicts: number; readOnly: boolean; onAbort: () => void; onChanges: () => void }) {
  return (
    <Notice
      tone="warn"
      icon="alert-triangle"
      className="git-stopped"
      action={
        readOnly ? undefined : (
          <span className="git-stopped-actions">
            <button type="button" className="btn sm" onClick={onChanges}>
              {t("Open Changes to resolve")}
            </button>
            <button type="button" className="btn sm danger" onClick={onAbort}>
              {tc("git", "Abort")}
            </button>
          </span>
        )
      }
    >
      {stoppedWords(operation, conflicts)}
    </Notice>
  );
}
