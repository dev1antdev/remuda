// What a new worktree got from its repository's .worktreeinclude, said in a toast once it is made. The pure part.

import { t, tn } from "@/lib/i18n";

// Included is the server's word on the copy (internal/worktrees.Included)
export type Included = { listed: boolean; copied?: string[]; skipped?: { path: string; why: string }[]; too_many?: number };

// FILES_NAMED is how many of the copied files the toast names
export const FILES_NAMED = 3;

// includedWords is the toast after a worktree is made: the files copied, a pattern list that matched too much or
// nothing, or why the copy failed; "" when the repository has no .worktreeinclude
export function includedWords(included?: Included, error?: string): string {
  if (error) return t("The worktree is made, but the files .worktreeinclude names were not copied: {error}", { error });
  if (!included?.listed) return "";
  if (included.too_many) return t(".worktreeinclude matches {n} ignored files, more than a worktree gets: nothing was copied — narrow its patterns", { n: included.too_many });
  const copied = included.copied ?? [];
  const skipped = included.skipped?.length ?? 0;
  if (!copied.length) return skipped ? tn(skipped, "Nothing copied from .worktreeinclude: {n} file is already in the worktree or leads out of it", "Nothing copied from .worktreeinclude: {n} files are already in the worktree or lead out of it") : t(".worktreeinclude matches no ignored file: nothing to copy");
  const named = copied.slice(0, FILES_NAMED).join(", ") + (copied.length > FILES_NAMED ? ` +${copied.length - FILES_NAMED}` : "");
  return tn(copied.length, "Copied {n} file from .worktreeinclude: {files}", "Copied {n} files from .worktreeinclude: {files}", { files: named });
}
