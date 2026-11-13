// The Git lens's menus, after git-graph's: what is under the pointer — a branch, a remote branch, a tag, a commit, the
// uncommitted row — decides the rows. Fetch, pull and push are not this lens's, nor deleting a branch on a remote.

import type { IconName } from "@/ui/icons";
import { t } from "@/lib/i18n";

export type Action =
  | { type: "checkout"; name: string }
  | { type: "checkout-remote"; name: string; branch: string }
  | { type: "checkout-commit"; hash: string }
  | { type: "branch-create"; hash: string }
  | { type: "branch-rename"; name: string }
  | { type: "branch-delete"; name: string }
  | { type: "merge"; ref?: string; hash?: string; what: string }
  | { type: "rebase"; ref?: string; hash?: string; what: string }
  | { type: "cherry-pick"; hash: string; merge: boolean }
  | { type: "revert"; hash: string; merge: boolean }
  | { type: "reset"; hash: string; uncommitted?: boolean }
  | { type: "tag-add"; hash: string }
  | { type: "tag-delete"; name: string }
  | { type: "copy"; text: string }
  | { type: "changes" };

export type Target =
  | { kind: "uncommitted" }
  | { kind: "branch"; name: string; hash: string }
  | { kind: "remote"; name: string; hash: string }
  | { kind: "tag"; name: string; hash: string }
  | { kind: "commit"; hash: string; subject: string; parents: string[] };

// Context is where the checkout stands as the graph last read it
export type Context = { branch: string; head: string; branches: string[]; dirty: number };

export type Row = { label: string; icon: IconName; action: Action; danger?: boolean } | { sep: true };

export const short = (hash: string) => hash.slice(0, 8);

// current is the branch checked out, or HEAD while it is detached
export const current = (ctx: Context) => ctx.branch || "HEAD";

const row = (label: string, icon: IconName, action: Action, danger = false): Row => ({ label, icon, action, ...(danger ? { danger } : {}) });

// joined lays the sections out with a separator between those that hold a row
function joined(sections: (Row | false)[][]): Row[] {
  const out: Row[] = [];
  for (const section of sections) {
    const rows = section.filter((r): r is Row => !!r);
    if (!rows.length) continue;
    if (out.length) out.push({ sep: true });
    out.push(...rows);
  }
  return out;
}

// menuFor is the rows of the menu for what was pressed
export function menuFor(target: Target, ctx: Context): Row[] {
  const cur = current(ctx);
  if (target.kind === "uncommitted") {
    return joined([[row(t("Open Changes"), "file", { type: "changes" })], [!!ctx.head && row(t("Reset uncommitted changes…"), "rotate-ccw", { type: "reset", hash: ctx.head, uncommitted: true })]]);
  }
  if (target.kind === "branch") {
    const name = target.name;
    const here = name === ctx.branch;
    return joined([
      [!here && row(t("Checkout branch"), "git-branch", { type: "checkout", name }), row(t("Rename branch…"), "edit", { type: "branch-rename", name }), !here && row(t("Delete branch…"), "trash", { type: "branch-delete", name }, true)],
      [
        !here && row(t("Merge into {current}…", { current: cur }), "git-merge", { type: "merge", ref: name, what: t("branch {name}", { name }) }),
        !here && row(t("Rebase {current} on this branch…", { current: cur }), "git-rebase", { type: "rebase", ref: name, what: t("branch {name}", { name }) }),
      ],
      [row(t("Copy branch name"), "copy", { type: "copy", text: name })],
    ]);
  }
  if (target.kind === "remote") {
    const name = target.name;
    const branch = name.slice(name.indexOf("/") + 1);
    const local = ctx.branches.includes(branch);
    return joined([
      [branch !== ctx.branch && (local ? row(t("Checkout {branch}", { branch }), "git-branch", { type: "checkout", name: branch }) : row(t("Checkout branch…"), "git-branch", { type: "checkout-remote", name, branch }))],
      [row(t("Merge into {current}…", { current: cur }), "git-merge", { type: "merge", ref: name, what: t("remote branch {name}", { name }) })],
      [row(t("Copy branch name"), "copy", { type: "copy", text: name })],
    ]);
  }
  if (target.kind === "tag") {
    return joined([[row(t("Delete tag…"), "trash", { type: "tag-delete", name: target.name }, true)], [row(t("Copy tag name"), "copy", { type: "copy", text: target.name })]]);
  }
  const hash = target.hash;
  const here = hash === ctx.head;
  const merge = target.parents.length > 1;
  const what = t("commit {hash}", { hash: short(hash) });
  return joined([
    [row(t("Add tag…"), "tag", { type: "tag-add", hash }), row(t("Create branch…"), "plus", { type: "branch-create", hash })],
    [!here && row(t("Checkout…"), "git-commit", { type: "checkout-commit", hash }), !here && row(t("Cherry pick…"), "cherry", { type: "cherry-pick", hash, merge }), row(t("Revert…"), "undo", { type: "revert", hash, merge })],
    [
      !here && row(t("Merge into {current}…", { current: cur }), "git-merge", { type: "merge", hash, what }),
      !here && row(t("Rebase {current} on this commit…", { current: cur }), "git-rebase", { type: "rebase", hash, what }),
      row(t("Reset {current} to this commit…", { current: cur }), "rotate-ccw", { type: "reset", hash }),
    ],
    [row(t("Copy commit hash"), "copy", { type: "copy", text: hash }), row(t("Copy commit subject"), "copy", { type: "copy", text: target.subject })],
  ]);
}
