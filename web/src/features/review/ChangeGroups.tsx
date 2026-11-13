// The uncommitted files in better-changes' groups — Conflicts, Staged, Unstaged, Untracked — each a header with its
// count and its one bulk button, its rows flat or as a tree, and the files unchanged since the last look folded at its
// foot; a row's counts give way to stage, unstage and discard under the pointer (always on a touch screen).

import type { CSSProperties, ReactNode } from "react";
import { t } from "@/lib/i18n";
import { plural } from "@/lib/text";
import { Icon, Tip } from "@/ui";
import type { IconName } from "@/ui/icons";
import type { RowLook } from "./Diff";
import { actionWords, bulkOf, bulkPaths, bulkWords, conflictWords, groupTitle, rowActions, type BulkAction, type ChangeRow, type GroupId, type RowAction } from "./groups";
import type { GroupLayout } from "./layout";
import type { TreeRow } from "./tree";

const ACTION_ICONS: Record<RowAction, IconName> = { stage: "plus", unstage: "minus", discard: "rotate-ccw" };

export function ChangeGroups({ layouts, tree, whyNot, writing, closed, onFolder, onUnfold, onBulk, onAction, fileRow, conflictBody }: {
  layouts: GroupLayout[];
  tree: boolean;
  whyNot: string; // why no write can run now, "" when they can
  writing: string | null; // the row or group a write is running for
  closed: ReadonlySet<string>;
  onFolder: (key: string) => void;
  onUnfold: (group: GroupId) => void;
  onBulk: (group: GroupId, action: BulkAction, paths: string[]) => void;
  onAction: (row: ChangeRow, action: RowAction) => void;
  fileRow: (row: ChangeRow, look: RowLook) => ReactNode;
  conflictBody: (row: ChangeRow) => ReactNode;
}) {
  const held = !!whyNot || writing !== null; // the buttons stay where they are, their tips saying why they do nothing now
  const actionsOf = (row: ChangeRow) => (
    <>
      {rowActions(row).map((action) => {
        const words = whyNot || actionWords(action, row);
        return (
          <Tip key={action} text={words}>
            <button type="button" className={`icon-btn sm review-act is-${action}`} aria-label={actionWords(action, row)} aria-disabled={held || undefined} onClick={held ? undefined : () => onAction(row, action)}>
              <Icon name={ACTION_ICONS[action]} size={12} />
            </button>
          </Tip>
        );
      })}
    </>
  );
  const item = (layout: GroupLayout, entry: TreeRow<ChangeRow>) => {
    if (entry.kind === "folder") {
      const key = `${layout.group}:${entry.path}`;
      const open = !closed.has(key);
      return (
        <button key={`folder:${key}`} type="button" className="review-folder" aria-expanded={open} style={{ "--depth": entry.depth } as CSSProperties} onClick={() => onFolder(key)}>
          <span className={`chevron${open ? " open" : ""}`}>
            <Icon name="chevron-right" size={12} />
          </span>
          <Icon name="folder" size={12} />
          <span className="review-folder-name">{entry.name}</span>
          <span className="badge quiet">{entry.count}</span>
        </button>
      );
    }
    const row = entry.item;
    return fileRow(row, {
      key: row.key,
      stat: row.stat,
      letter: row.group === "untracked" || row.group === "conflict" ? undefined : row.letter,
      conflict: row.group === "conflict" ? conflictWords(row.file.conflict) : undefined,
      actions: actionsOf(row),
      depth: tree ? entry.depth : undefined,
      name: tree ? entry.name : undefined,
      body: row.group === "conflict" ? () => conflictBody(row) : undefined,
      from: row.from,
    });
  };
  return (
    <>
      {layouts.map((layout) => {
        const bulk = bulkOf(layout.group);
        const rows = [...layout.items, ...layout.unchangedItems].flatMap((entry) => (entry.kind === "file" ? [entry.item] : []));
        return (
          <section key={layout.group} className={`review-group is-${layout.group}`} aria-label={groupTitle(layout.group)}>
            <div className="review-group-head">
              <span className="review-group-title">{groupTitle(layout.group)}</span>
              <span className="badge quiet">{layout.total}</span>
              {bulk && (
                <Tip text={whyNot || bulkWords(bulk)}>
                  <button type="button" className="linkish review-bulk" aria-disabled={held || undefined} onClick={held ? undefined : () => onBulk(layout.group, bulk, bulkPaths(rows))}>
                    <Icon name={bulk === "stage" ? "plus" : "minus"} size={12} /> {bulkWords(bulk)}
                  </button>
                </Tip>
              )}
            </div>
            {layout.items.map((entry) => item(layout, entry))}
            {layout.unchanged > 0 && (
              <div className="review-fold">
                <button type="button" className="linkish" aria-expanded={layout.unchangedShown} onClick={() => onUnfold(layout.group)}>
                  <Icon name={layout.unchangedShown ? "chevron-down" : "chevron-right"} size={12} /> {t("{files} unchanged since you looked", { files: plural(layout.unchanged, "file") })}
                </button>
                {layout.unchangedShown && layout.unchangedItems.map((entry) => item(layout, entry))}
              </div>
            )}
          </section>
        );
      })}
    </>
  );
}
