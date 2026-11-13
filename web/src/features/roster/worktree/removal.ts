// The remove dialog's table of consequences, after Orca's: the folder deleted or kept, the branch kept or deleted as the
// person flips it, and in red what would be lost. Pure.

import { tn, t } from "@/lib/i18n";
import { tilde } from "./sources";
import type { Removal } from "./api";

// Row is one line of the table: what it is, its name, where, and the pill
export type Row = { what: "folder" | "branch"; label: string; value: string; under?: string; pill: "Deleted" | "Kept"; flips: boolean };

// sharedHere says another workspace uses the checkout: then only this workspace closes, the folder and branch stay
export const sharedHere = (rm: Removal): boolean => (rm.shared?.length ?? 0) > 0;

// branchGoesByDefault is the branch's pill before the person flips it: Deleted once merged, as a close always took it
export const branchGoesByDefault = (rm: Removal): boolean => !!rm.branch && !sharedHere(rm) && (rm.merged || !!rm.merged_pr);

export function rows(rm: Removal, deleteBranch: boolean, home: string): Row[] {
  const shared = sharedHere(rm);
  const name = rm.path.split(/[\\/]/).filter(Boolean).pop() ?? rm.path;
  const parent = rm.path.slice(0, rm.path.length - name.length);
  const out: Row[] = [{ what: "folder", label: t("Folder"), value: name, under: tilde(parent, home), pill: shared ? "Kept" : "Deleted", flips: false }];
  if (rm.branch) out.push({ what: "branch", label: t("Branch"), value: rm.branch, pill: deleteBranch && !shared ? "Deleted" : "Kept", flips: !shared });
  return out;
}

// warnings are what would be lost, each a sentence in red
export function warnings(rm: Removal, deleteBranch: boolean): string[] {
  if (sharedHere(rm)) return [];
  const out: string[] = [];
  if (rm.dirty > 0) out.push(t("Uncommitted changes in this worktree will be lost."));
  if (deleteBranch && rm.branch && rm.unmerged > 0)
    out.push(tn(rm.unmerged, "{n} commit on {branch} is on no remote and in no other branch: deleting the branch loses it.", "{n} commits on {branch} are on no remote and in no other branch: deleting the branch loses them.", { branch: rm.branch }));
  for (const sub of rm.submodules ?? []) out.push(t("Submodule {name} has commits that aren't pushed anywhere: they will be lost.", { name: sub }));
  return out;
}

// sharedWords is the sentence a shared checkout's dialog says instead
export function sharedWords(rm: Removal): string {
  const names = (rm.shared ?? []).join(", ");
  return tn(rm.shared?.length ?? 0, "{names} also uses this worktree, so it stays. Only this workspace closes.", "{names} also use this worktree, so it stays. Only this workspace closes.", { names });
}

// buttonWords is the danger button's: Remove, Remove anyway past a loss, Close workspace for a shared checkout
export function buttonWords(rm: Removal, deleteBranch: boolean, busy: boolean): string {
  if (busy) return t("Removing…");
  if (sharedHere(rm)) return t("Close workspace");
  return warnings(rm, deleteBranch).length > 0 ? t("Remove anyway") : t("Remove");
}

// blockedWords is why nothing may go now: an agent at work there
export function blockedWords(rm: Removal): string {
  return rm.working && !sharedHere(rm) ? t("{why}: let it finish, or stop it, before its worktree goes.", { why: rm.working }) : "";
}
