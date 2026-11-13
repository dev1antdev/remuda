// One folder of the project tree, read when it is opened and again at every tick of the tree; an open folder under it
// is a folder of its own, its rows drawn under a guide line at its parent's chevron.

import { useEffect, useRef } from "react";
import { useQuery } from "@/lib/query";
import type { MenuItem } from "@/ui";
import { t } from "@/lib/i18n";
import { fsTree } from "../api";
import TreeRow from "./TreeRow";
import { childOf, INDENT, moreWords, type TreeEntry } from "./tree";

export type TreeContext = {
  machineId: string;
  dir: string;
  active: boolean;
  tick: number; // moves when the tree reads its open folders again
  isOpen: (rel: string) => boolean;
  toggle: (rel: string) => void;
  open: (entry: TreeEntry, rel: string) => void;
  items: (entry: TreeEntry, rel: string) => MenuItem[];
};

export default function TreeFolder({ rel, depth, ctx }: { rel: string; depth: number; ctx: TreeContext }) {
  const { data, error, loading, reload } = useQuery(() => fsTree(ctx.machineId, ctx.dir, rel), [ctx.machineId, ctx.dir, rel], { enabled: ctx.active, keep: true });
  const seen = useRef(ctx.tick);
  useEffect(() => {
    if (seen.current === ctx.tick) return;
    seen.current = ctx.tick;
    reload();
  }, [ctx.tick, reload]);
  const guide = { "--guide": `${4 + depth * INDENT - INDENT / 2}px` } as React.CSSProperties;
  if (loading && !data)
    return (
      <div className={depth ? "tree-group nested" : "tree-group"} style={depth ? guide : undefined} aria-busy="true">
        {[60, 44, 72].map((width, i) => (
          <div key={i} className="tree-skeleton" style={{ marginLeft: 4 + depth * INDENT + 20, width: `${width}%` }} />
        ))}
      </div>
    );
  if (error && !data) return <div className="tree-note is-error" style={{ paddingLeft: 4 + depth * INDENT + 20 }}>{t("This folder could not be read: {error}", { error })}</div>;
  if (!data) return null;
  return (
    <div className={depth ? "tree-group nested" : "tree-group"} style={depth ? guide : undefined}>
      {data.entries.length === 0 && <div className="tree-note" style={{ paddingLeft: 4 + depth * INDENT + 20 }}>{t("This folder is empty.")}</div>}
      {data.entries.map((entry) => {
        const path = childOf(rel, entry.name);
        const open = !!entry.dir && !entry.gone && ctx.isOpen(path);
        return (
          <div key={entry.name} className="tree-item">
            <TreeRow entry={entry} rel={path} depth={depth} open={open} onClick={() => (entry.dir ? !entry.gone && ctx.toggle(path) : ctx.open(entry, path))} items={() => ctx.items(entry, path)} />
            {open && <TreeFolder rel={path} depth={depth + 1} ctx={ctx} />}
          </div>
        );
      })}
      {data.more && <div className="tree-note" style={{ paddingLeft: 4 + depth * INDENT + 20 }}>{moreWords()}</div>}
      {depth === 0 && data.git && <div className="tree-note">{t("Git could not read this folder: {error}", { error: data.git })}</div>}
    </div>
  );
}
