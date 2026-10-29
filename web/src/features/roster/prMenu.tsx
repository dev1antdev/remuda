// The pull request's menu, shared by a wall tile's chips and the workspace's ⋯ menu: Open, then the hand-offs its state
// allows — the failed checks' log, the reviewers' comments — each a message the server composes with gh on the machine
// and sends to the pane's agent, a toast when it went, the server's word when it could not; and, for a merged worktree,
// its removal behind a confirm.

import { act } from "@/app/act";
import { actions } from "@/app/store";
import { openUrl } from "@/lib/host";
import { errMsg } from "@/lib/text";
import type { MenuItem } from "@/ui";
import { handFailure, removeWorktree, sendReview, type Machine, type PullRequest, type Session } from "./api";
import { actionLabel, actionsFor, handedWords, removeAsk, removedWords, type PRAction } from "./prActions";
import { prWords } from "./prStatus";
import { t } from "@/lib/i18n";

// handPR sends one hand-off: the server reads GitHub with gh on the machine and submits the message to the agent.
export async function handPR(action: PRAction, machineId: string, paneId: string, label: string): Promise<void> {
  actions.notify(action === "failure" ? t("Reading the failed checks of {pr}…", { pr: label }) : t("Reading the reviews of {pr}…", { pr: label }), { brief: true });
  try {
    await (action === "failure" ? handFailure(machineId, paneId) : sendReview(machineId, paneId));
    actions.notify(handedWords(action, label));
  } catch (e) {
    actions.notify(errMsg(e), { error: true });
  }
}

// askRemoveWorktree is the confirm before a merged worktree and its branch go; a yes closes the workspace and removes them.
export function askRemoveWorktree(machine: Machine, session: Session): void {
  actions.ask(removeAsk(session.name), t("Remove the worktree"), () => void act(removeWorktree(machine.id, session.id).then((r) => actions.notify(removedWords(r.branch)))));
}

// prMenuItems are the rows behind a tile's chips: Open with the state as its hint, then the hand-offs the state allows.
export function prMenuItems(machineId: string, paneId: string, pr: PullRequest): MenuItem[] {
  const items: MenuItem[] = [{ label: t("Open {pr}", { pr: pr.label }), icon: "external-link", hint: pr.status ? prWords(pr.status) : undefined, onSelect: () => openUrl(pr.url) }];
  const hands = actionsFor(pr);
  if (hands.length > 0) items.push({ sep: true });
  for (const action of hands) {
    items.push({ label: actionLabel(action, pr.label), icon: action === "failure" ? "alert-triangle" : "git-branch", onSelect: () => void handPR(action, machineId, paneId, pr.label) });
  }
  return items;
}
