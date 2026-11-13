// The file list as a tree, as gitview draws one: folders first by name, then the files, a folder holding only one
// folder folded into one row ("src/app/"), each folder with how many files it holds; a closed folder hides what is under it.

export type TreeRow<T> = { kind: "folder"; path: string; name: string; depth: number; count: number } | { kind: "file"; item: T; name: string; depth: number };

type Folder<T> = { folders: Map<string, Folder<T>>; files: { item: T; name: string }[]; count: number };

const folder = <T,>(): Folder<T> => ({ folders: new Map(), files: [], count: 0 });

// treeRows lays the items out as a tree by their path, the folders' own paths in closed hiding their insides
export function treeRows<T>(items: T[], pathOf: (item: T) => string, closed: ReadonlySet<string> = new Set()): TreeRow<T>[] {
  const root = folder<T>();
  for (const item of items) {
    const parts = pathOf(item).split("/");
    const name = parts.pop()!;
    let node = root;
    node.count++;
    for (const part of parts) {
      let next = node.folders.get(part);
      if (!next) node.folders.set(part, (next = folder<T>()));
      next.count++;
      node = next;
    }
    node.files.push({ item, name });
  }
  const rows: TreeRow<T>[] = [];
  const walk = (node: Folder<T>, prefix: string, depth: number) => {
    for (const [part, child] of [...node.folders].sort(([a], [b]) => a.localeCompare(b))) {
      let name = part;
      let inside = child;
      while (inside.files.length === 0 && inside.folders.size === 1) {
        const [only, next] = [...inside.folders][0]!;
        name = `${name}/${only}`;
        inside = next;
      }
      const path = prefix + name;
      rows.push({ kind: "folder", path, name: `${name}/`, depth, count: inside.count });
      if (!closed.has(path)) walk(inside, `${path}/`, depth + 1);
    }
    for (const file of node.files) rows.push({ kind: "file", item: file.item, name: file.name, depth });
  };
  walk(root, "", 0);
  return rows;
}
