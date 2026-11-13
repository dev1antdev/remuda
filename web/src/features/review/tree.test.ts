import { describe, expect, it } from "vitest";
import { treeRows } from "./tree";

const paths = ["src/app/main.go", "src/app/util/strings.go", "src/README.md", "docs/guide/intro.md", "go.mod"];
const shape = (rows: ReturnType<typeof treeRows<string>>) => rows.map((row) => `${"  ".repeat(row.depth)}${row.kind === "folder" ? `${row.name} (${row.count})` : row.name}`);

describe("the tree", () => {
  it("lays folders before files, folds a folder holding only one, and counts what each holds", () => {
    expect(shape(treeRows(paths, (path) => path))).toEqual([
      "docs/guide/ (1)",
      "  intro.md",
      "src/ (3)",
      "  app/ (2)",
      "    util/ (1)",
      "      strings.go",
      "    main.go",
      "  README.md",
      "go.mod",
    ]);
  });

  it("hides what a closed folder holds, by its folded path", () => {
    const rows = treeRows(paths, (path) => path, new Set(["docs/guide", "src/app"]));
    expect(shape(rows)).toEqual(["docs/guide/ (1)", "src/ (3)", "  app/ (2)", "  README.md", "go.mod"]);
    expect(rows.filter((row) => row.kind === "folder").map((row) => (row.kind === "folder" ? row.path : ""))).toEqual(["docs/guide", "src", "src/app"]);
  });

  it("keeps the items it was given", () => {
    const items = [{ path: "a/b.go", id: 1 }];
    const [, leaf] = treeRows(items, (item) => item.path);
    expect(leaf?.kind === "file" && leaf.item.id).toBe(1);
  });
});
