// One row of the project tree: the chevron of a folder, its icon, its name in git's colour, the mark's letter, the dot of a
// folder holding changes, and its menu behind ⋯ or a right-click. 26 px tall, indented by its depth.

import { useState } from "react";
import { FileIcon } from "@/ui/FileIcon";
import { Icon, Menu, type MenuItem } from "@/ui";
import { t } from "@/lib/i18n";
import { rowTitle, toneOf, INDENT, type TreeEntry } from "./tree";

export default function TreeRow({ entry, rel, depth, open, onClick, items }: {
  entry: TreeEntry;
  rel: string;
  depth: number;
  open: boolean;
  onClick: () => void;
  items: () => MenuItem[];
}) {
  const [menu, setMenu] = useState(false);
  const tone = toneOf(entry);
  const expands = !!entry.dir && !entry.gone;
  const classes = ["tree-row", tone && `tone-${tone}`, entry.ignored && "is-ignored", entry.gone && "is-gone", menu && "is-menu"].filter(Boolean).join(" ");
  return (
    <div
      className={classes}
      style={{ paddingLeft: 4 + depth * INDENT }}
      onContextMenu={(e) => {
        e.preventDefault();
        setMenu(true);
      }}
    >
      <button type="button" className="linkish tree-open" title={rowTitle(entry, rel)} aria-expanded={expands ? open : undefined} onClick={onClick}>
        <span className="tree-chevron">{expands && <Icon name={open ? "chevron-down" : "chevron-right"} size={12} />}</span>
        <FileIcon path={rel} dir={!!entry.dir} open={open} className="tree-icon" />
        <span className="tree-name">{entry.name}</span>
        {entry.link && <Icon name="link" size={12} className="tree-link" />}
      </button>
      <span className="tree-mark" aria-hidden="true">
        {entry.mark}
      </span>
      <span className={entry.dir && entry.changes ? "tree-dot on" : "tree-dot"} aria-hidden="true" />
      <Menu
        open={menu}
        onOpenChange={setMenu}
        tip={t("More")}
        label={entry.name}
        items={items}
        trigger={
          <button type="button" className="icon-btn sm tree-more" aria-label={t("More: {name}", { name: entry.name })}>
            <Icon name="more" size={14} />
          </button>
        }
      />
    </div>
  );
}
