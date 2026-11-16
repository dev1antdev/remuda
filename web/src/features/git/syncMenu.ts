// The Git lens's fetch, pull and push, in one menu at its head: the Repositories place's own three on the checkout the
// lens draws, and the push again without the repository's pre-push hook. Pull and push take the branch checked out, so
// a detached HEAD offers the fetch alone.

import type { SyncAction } from "@/features/repos/syncApi";
import { t } from "@/lib/i18n";
import type { IconName, MenuItem } from "@/ui";

const ICONS: Record<SyncAction, IconName> = { fetch: "refresh-cw", pull: "arrow-down-to-line", push: "arrow-up-from-line" };

// syncRows is the menu's rows for the branch checked out ("" while detached); running is the action under way, if any;
// onRun hears noVerify for the push without the pre-push hook
export function syncRows(branch: string, running: SyncAction | "", onRun: (action: SyncAction, noVerify: boolean) => void): MenuItem[] {
  const row = (action: SyncAction, label: string, hint: string, needsBranch: boolean, noVerify = false): MenuItem => ({
    label,
    icon: ICONS[action],
    hint,
    disabled: !!running || (needsBranch && !branch),
    onSelect: () => onRun(action, noVerify),
  });
  const pushHint = (words: string) => (branch ? words : t("no branch checked out"));
  return [
    row("fetch", t("Fetch"), t("the remote's branches read again; nothing here changes"), false),
    row("pull", t("Pull"), branch ? t("{branch} fast-forwarded to its upstream", { branch }) : t("no branch checked out"), true),
    row("push", t("Push"), pushHint(t("{branch} sent to its upstream, never forced", { branch })), true),
    row("push", t("Push without hooks"), pushHint(t("{branch} sent with --no-verify: the pre-push hook does not run", { branch })), true, true),
  ];
}
