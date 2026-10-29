// The bar under the list or the board while tasks are selected: the count, Move to…, Add label…, Start agents…, Mark done, Delete…, ×.

import { Icon, Menu, Tip, type MenuItem } from "@/ui";
import type { Label } from "./api";
import type { Action, ColumnChoice } from "./bulk";
import { selectionWords } from "./selection";
import { t } from "@/lib/i18n";

export function BulkBar({ count, busy, columns, labels, onAct, onStart, onClear }: {
  count: number;
  busy: boolean;
  columns: ColumnChoice[];
  labels: { label: Label; lacking: number }[];
  onAct: (action: Action) => void;
  onStart: () => void; // an agent on every selected task, through one dialog
  onClear: () => void;
}) {
  const partial = (n: number) => (n < count ? t("{n} of {count}", { n, count }) : undefined);
  const moveItems: MenuItem[] = columns.map((c) => ({
    label: c.title,
    hint: partial(c.have),
    onSelect: () => onAct(c.statusId !== undefined ? { kind: "move", statusId: c.statusId } : { kind: "move-named", column: c.title }),
  }));
  const labelItems: MenuItem[] = labels.length
    ? labels.map(({ label, lacking }) => ({ label: label.title, hint: partial(lacking), disabled: lacking === 0, onSelect: () => onAct({ kind: "label", labelId: label.id }) }))
    : [{ label: t("No labels yet"), disabled: true, onSelect: () => {} }];
  return (
    <div className="bulk-bar" role="toolbar" aria-label={t("Selected tasks")}>
      <span className="bulk-count">{selectionWords(count)}</span>
      <Menu
        label={t("Move to")}
        align="start"
        side="top"
        items={moveItems}
        trigger={
          <button type="button" className="btn sm" disabled={busy || columns.length === 0}>
            <Icon name="columns" size={14} /> {t("Move to…")}
          </button>
        }
      />
      <Menu
        label={t("Add label")}
        align="start"
        side="top"
        items={labelItems}
        trigger={
          <button type="button" className="btn sm" disabled={busy}>
            <Icon name="tag" size={14} /> {t("Add label…")}
          </button>
        }
      />
      <button type="button" className="btn sm" disabled={busy} onClick={onStart}>
        <Icon name="git-branch" size={14} /> {t("Start agents…")}
      </button>
      <button type="button" className="btn sm" disabled={busy} onClick={() => onAct({ kind: "done" })}>
        <Icon name="check" size={14} /> {t("Mark done")}
      </button>
      <button type="button" className="btn sm danger soft" disabled={busy} onClick={() => onAct({ kind: "delete" })}>
        <Icon name="trash" size={14} /> {t("Delete…")}
      </button>
      <Tip text="Clear the selection (Esc)">
        <button type="button" className="icon-btn sm bulk-clear" aria-label={t("Clear the selection")} disabled={busy} onClick={onClear}>
          <Icon name="x" size={14} />
        </button>
      </Tip>
    </div>
  );
}
