// How the groups lie on screen: each group's rows flat or as a tree, the files unchanged since the last look folded at
// the group's foot until asked for, and the rows the keys walk in the order they are drawn.

import { GROUPS, type ChangeRow, type GroupId } from "./groups";
import { treeRows, type TreeRow } from "./tree";

export type GroupLayout = {
  group: GroupId;
  total: number;
  items: TreeRow<ChangeRow>[];
  unchanged: number; // the rows unchanged since the last look, folded under the group
  unchangedShown: boolean;
  unchangedItems: TreeRow<ChangeRow>[];
};

// LayoutAsk is what the person chose: a tree, the folders closed ("group:folder"), whether there was a look to compare
// with, and the groups whose unchanged files are shown
export type LayoutAsk = { tree: boolean; closed: ReadonlySet<string>; looked: boolean; unfolded: ReadonlySet<GroupId> };

const flat = (rows: ChangeRow[]): TreeRow<ChangeRow>[] => rows.map((row) => ({ kind: "file", item: row, name: "", depth: 0 }));

function lay(rows: ChangeRow[], group: GroupId, ask: LayoutAsk): TreeRow<ChangeRow>[] {
  if (!ask.tree) return flat(rows);
  const closed = new Set([...ask.closed].filter((key) => key.startsWith(`${group}:`)).map((key) => key.slice(group.length + 1)));
  return treeRows(rows, (row) => row.file.path, closed);
}

// layoutGroups lays out every group that has rows, in their order
export function layoutGroups(groups: Record<GroupId, ChangeRow[]>, ask: LayoutAsk): GroupLayout[] {
  return GROUPS.filter((group) => groups[group].length > 0).map((group) => {
    const rows = groups[group];
    const fresh = ask.looked ? rows.filter((row) => row.file.fresh) : rows;
    const unchanged = ask.looked ? rows.filter((row) => !row.file.fresh) : [];
    return { group, total: rows.length, items: lay(fresh, group, ask), unchanged: unchanged.length, unchangedShown: ask.unfolded.has(group), unchangedItems: lay(unchanged, group, ask) };
  });
}

// visibleKeys is every row drawn, in order: what j and k walk
export const visibleKeys = (layouts: GroupLayout[]): string[] =>
  layouts.flatMap((layout) => [...layout.items, ...(layout.unchangedShown ? layout.unchangedItems : [])].flatMap((item) => (item.kind === "file" ? [item.item.key] : [])));
