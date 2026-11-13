// Whether a workspace's folder is a git checkout, as the roster tells: herdr names its worktree, or the review's sweep read
// a branch there (a short hash when HEAD is detached). The Git lens's button is there either way, disabled without one.

import type { Session } from "@/features/roster/api";
import { t } from "@/lib/i18n";

export const isCheckout = (session: Pick<Session, "branch" | "worktree">): boolean => !!(session.branch || session.worktree);

// gitLensTip is what the lens's button says it shows, or why it shows nothing here
export const gitLensTip = (checkout: boolean): string => (checkout ? t("This checkout's history: every branch as a lane, its commits and their files") : t("This workspace's folder is not a git checkout"));
