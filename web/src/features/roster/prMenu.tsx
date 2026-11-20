// The pull request's menu, shared by the sidebar card's badge and the workspace's ⋯ menu: Open, then the hand-offs its state
// allows — the failed checks' log, the reviewers' comments — each a message the server composes with gh (glab for a
// GitLab merge request) on the machine
// and sends to the pane's agent, a toast when it went, the server's word when it could not; and, for a merged worktree,
// its removal through the remove dialog.

import { actions } from "@/app/store";
import { openUrl } from "@/lib/host";
import { errMsg } from "@/lib/text";
import type { MenuItem } from "@/ui";
import { handFailure, sendReview, type Machine, type PullRequest, type Session } from "./api";
import { actionLabel, actionsFor, handedWords, type PRAction } from "./prActions";
import { prWords } from "./prStatus";
import { isPullURL } from "@/features/pr/links";
import { t } from "@/lib/i18n";

// handPR sends one hand-off: the server reads GitHub with gh, or GitLab with glab, on the machine and submits the
// message to the agent.
export async function handPR(action: PRAction, machineId: string, paneId: string, label: string): Promise<void> {
  actions.notify(action === "failure" ? t("Reading the failed checks of {pr}…", { pr: label }) : t("Reading the reviews of {pr}…", { pr: label }), { brief: true });
  try {
    await (action === "failure" ? handFailure(machineId, paneId) : sendReview(machineId, paneId));
    actions.notify(handedWords(action, label));
  } catch (e) {
    actions.notify(errMsg(e), { error: true });
  }
}

// askRemoveWorktree opens the remove dialog: what goes, what would be lost, and the server's checks when it is confirmed.
export function askRemoveWorktree(machine: Machine, session: Session): void {
  actions.openDialog({ kind: "remove-worktree", machineId: machine.id, session });
}

// prPanelRow opens the pull request panel on a GitHub pull request, in the checkout it came from; none for a merge request
export function prPanelRow(machineId: string, paneId: string, pr: PullRequest, dir: string): MenuItem[] {
  if (!isPullURL(pr.url) || !dir) return [];
  return [{ label: t("Pull request panel"), icon: "git-pull-request", hint: t("comments, checks, files, merge"), onSelect: () => actions.openPRPanel({ machineId, dir, paneId, url: pr.url }) }];
}

// prMenuItems are the rows behind a card's badge: Open with the state as its hint, then the pull request panel and the
// hand-offs the state allows (a viewer's menu ends before them).
export function prMenuItems(machineId: string, paneId: string, pr: PullRequest, dir = ""): MenuItem[] {
  const items: MenuItem[] = [{ label: t("Open {pr}", { pr: pr.label }), icon: "external-link", hint: pr.status ? prWords(pr.status) : undefined, onSelect: () => openUrl(pr.url) }];
  const hands = actionsFor(pr);
  const panel = prPanelRow(machineId, paneId, pr, dir);
  if (hands.length > 0 || panel.length > 0) items.push({ sep: true }, ...panel);
  for (const action of hands) {
    items.push({ label: actionLabel(action, pr.label), icon: action === "failure" ? "alert-triangle" : "git-branch", onSelect: () => void handPR(action, machineId, paneId, pr.label) });
  }
  return items;
}
