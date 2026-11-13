// The lens's other scope, after gitview: the branch's whole work against the branch it was cut from — its commits and
// what the working tree holds beyond them — read only, flat or as a tree; comments still go to the agent.

import type { CSSProperties, ReactNode } from "react";
import { t } from "@/lib/i18n";
import { Icon } from "@/ui";
import type { BaseView, ReviewFile } from "./api";
import type { RowLook } from "./Diff";
import { treeRows } from "./tree";

export const baseKey = (path: string) => `base:${path}`;
const FOLDER = "base:"; // the closed folders of this scope, among the lens's own

// baseOrder is the base scope's rows as the keys walk them, a closed folder's files left out
export function baseOrder(view: BaseView | null | undefined, tree: boolean, closed: ReadonlySet<string>): string[] {
  if (!view) return [];
  if (!tree) return view.files.map((file) => baseKey(file.path));
  return treeRows(view.files, (file) => file.path, closedHere(closed)).flatMap((row) => (row.kind === "file" ? [baseKey(row.item.path)] : []));
}

const closedHere = (closed: ReadonlySet<string>) => new Set([...closed].filter((key) => key.startsWith(FOLDER)).map((key) => key.slice(FOLDER.length)));

export function BaseFiles({ view, error, tree, closed, onFolder, fileRow }: {
  view: BaseView | null;
  error: string;
  tree: boolean;
  closed: ReadonlySet<string>;
  onFolder: (key: string) => void;
  fileRow: (file: ReviewFile, look: RowLook) => ReactNode;
}) {
  if (error) return <div className="empty">{error}</div>;
  if (!view) return <div className="empty">{t("Finding the branch it was cut from…")}</div>;
  if (!view.base) return <div className="empty">{t("No branch to compare with: this branch has no remote and was cut from no main branch here.")}</div>;
  if (view.files.length === 0) return <div className="empty">{t("Nothing here differs from {base}.", { base: view.base })}</div>;
  const files = view.files.map((file) => ({ ...file, fresh: false }));
  const look = (file: ReviewFile, depth?: number, name?: string): RowLook => ({ key: baseKey(file.path), depth, name, from: { base: view.merge_base } });
  if (!tree) return <section className="review-group is-base">{files.map((file) => fileRow(file, look(file)))}</section>;
  const shut = closedHere(closed);
  return (
    <section className="review-group is-base">
      {treeRows(files, (file) => file.path, shut).map((row) =>
        row.kind === "folder" ? (
          <button key={`folder:${row.path}`} type="button" className="review-folder" aria-expanded={!shut.has(row.path)} style={{ "--depth": row.depth } as CSSProperties} onClick={() => onFolder(FOLDER + row.path)}>
            <span className={`chevron${shut.has(row.path) ? "" : " open"}`}>
              <Icon name="chevron-right" size={12} />
            </span>
            <Icon name="folder" size={12} />
            <span className="review-folder-name">{row.name}</span>
            <span className="badge quiet">{row.count}</span>
          </button>
        ) : (
          fileRow(row.item, look(row.item, row.depth, row.name))
        ),
      )}
    </section>
  );
}
