// The workspace's ⋯ menu and the folder's: a tab, a workspace in the same folder, a worktree, the renames, the folder's
// icon and its screen-sharing mark, the pull request the agent named with what its state allows — its failed checks or
// its review handed to the agent, a merged worktree removed — the agent forked, restarted on its session or moved to
// another machine, put to sleep or woken, the mutes, close. Rows for `ui/Menu`, worked out when a menu opens.

import { act } from "@/app/act";
import { actions } from "@/app/store";
import { offerReopen } from "@/features/reopen/offer";
import { agentTitle, askRestart } from "@/features/updates/restartPane";
import { canFork, forkLabel } from "@/features/chat/fork";
import { forkPane } from "@/features/chat/forkPane";
import { sleepRows } from "@/features/sleep/menu";
import { openUrl } from "@/lib/host";
import type { MenuItem } from "@/ui";
import { killSession, muteWorkspace, unmuteWorkspace, type Machine, type Session } from "./api";
import { leftWords, muteChoices, mutedUntil } from "./mute";
import { actionLabel, actionsFor, canRemoveWorktree } from "./prActions";
import { askRemoveWorktree, handPR } from "./prMenu";
import { cardDetailsRows, cardNoteRows, folderNoteRows } from "./cardMenu";
import { prWords } from "./prStatus";
import { isPullURL } from "@/features/pr/links";
import { closeAsk, madeWorktree, tidyWords } from "./tidy";
import { folderOf, repositoryOf, type Folder } from "./workspaces";
import { markedFolder, sharingOf, SHARING_KEY, DEFAULT_SHARING } from "./sharing";
import { toggleSharingFolder } from "./useSharing";
import { readPref } from "@/lib/prefs";
import { onWindows } from "./system";
import { basename } from "@/lib/text";
import { t, tn } from "@/lib/i18n";

// closeWorkspace asks before a workspace closes; one on a worktree herdr or remuda made gets the remove dialog, which
// says what of the worktree goes with it and what would be lost
export function closeWorkspace(m: Machine, s: Session) {
  if (madeWorktree(s.worktree) && m.state === "connected") return actions.openDialog({ kind: "remove-worktree", machineId: m.id, session: s });
  const n = s.tabs.reduce((a, t) => a + t.panes.length, 0);
  const agents = s.tabs.flatMap((tab) => tab.panes.filter((p) => p.agent).map((p) => p.id));
  const panes = tn(n, "Close “{name}”? {n} pane closes and the agent in it stops.", "Close “{name}”? {n} panes close and the agents in them stop.", { name: s.name });
  const close = async () => {
    const answer = await killSession(m.id, s.id);
    if (answer?.worktree) {
      const said = tidyWords(answer.worktree);
      actions.notify(said.text, { error: said.error });
    }
  };
  actions.ask(closeAsk(panes, madeWorktree(s.worktree)), t("Close workspace"), () => void act(close()).then(() => offerReopen(m.id, agents)));
}

// Shown is how the row on screen names its folder: under the screen-sharing mode (masked) by its neutral name, and the
// menu then says no name of the person's own either
export type Shown = { name: string; masked: boolean };

// folderRows are what a folder itself offers: a new workspace — in a repository a worktree of its own, a new folder and
// branch, since a second workspace on the same checkout is what a tab already is; elsewhere one in the folder — its
// icon (not while the screen-sharing mode hides it: the icon is not on screen then, and its dialog names the folder),
// its screen-sharing mark, the editor on it
function folderRows(m: Machine, folder: string, name: string, icon: Session["icon"], repository: Session | null, shown: Shown): MenuItem[] {
  const connected = m.state === "connected";
  const hidden = markedFolder(sharingOf(readPref(SHARING_KEY, DEFAULT_SHARING)), m.id, folder);
  const newWorkspace: MenuItem = repository
    ? { label: t("New workspace…"), icon: "git-branch", hint: t("a worktree of its own: a branch, an issue, a pull request or a new branch"), disabled: !connected, onSelect: () => actions.openDialog({ kind: "worktree", machineId: m.id, session: repository }) }
    : { label: t("New workspace in this folder…"), icon: "plus", hint: shown.masked ? shown.name : name, disabled: !connected, onSelect: () => actions.openDialog({ kind: "workspace", machineId: m.id, dir: folder }) };
  const settings: MenuItem[] = repository ? [{ label: t("Project settings…"), icon: "settings", hint: t("new worktrees, their setup script, the run actions"), onSelect: () => actions.openDialog({ kind: "project-settings", machineId: m.id, root: folder }) }] : [];
  return [
    newWorkspace,
    ...settings,
    ...(shown.masked ? [] : [{ label: t("Folder icon…"), icon: "folder" as const, onSelect: () => actions.openDialog({ kind: "folder-icon", machineId: m.id, folder, name, icon }) }]),
    { label: t("Hide when sharing the screen"), icon: "eye-off", checked: hidden, onSelect: () => toggleSharingFolder(m.id, folder) },
    ...(onWindows(m) ? [] : [{ label: t("Open in the editor"), icon: "code" as const, hint: t("the Code mode on this folder"), disabled: !connected, onSelect: () => actions.openInCode(m.id, folder, folder) }]), // a Windows PC has no code-server
  ];
}

// folderMenuItems is the folder line's ⋯
export function folderMenuItems(m: Machine, folder: Folder, shown: Shown): MenuItem[] {
  const rows = folderRows(m, folder.path, folder.name, folder.icon, repositoryOf(folder), shown);
  return [...rows.slice(0, 1), ...folderNoteRows(m, folder, shown.masked), ...rows.slice(1)];
}

// workspaceMenuItems is a workspace card's ⋯: its own rows, then its folder's
export function workspaceMenuItems(m: Machine, session: Session, shown: Shown): MenuItem[] {
  const connected = m.state === "connected";
  const tab = session.tabs.find((x) => x.id === session.active_tab_id) ?? session.tabs[0];
  const chat = tab?.panes.find((p) => p.agent);
  const pr = chat?.pr;
  const muted = mutedUntil(session);
  const folder = folderOf(session);
  const rows: MenuItem[] = [
    ...cardDetailsRows(m, session, shown.masked, chat?.id),
    { label: t("New tab…"), icon: "plus", hint: t("another agent or a terminal in the same folder"), disabled: !connected, onSelect: () => actions.openDialog({ kind: "tab", machineId: m.id, session }) },
    { label: t("Rename workspace…"), icon: "edit", onSelect: () => actions.openDialog({ kind: "rename", machineId: m.id, what: "workspace", id: session.id, label: session.name }) },
  ];
  if (tab) rows.push({ label: t("Rename tab {tab}…", { tab: tab.named && !shown.masked ? `“${tab.label}”` : tab.number }), icon: "tag", onSelect: () => actions.openDialog({ kind: "rename", machineId: m.id, what: "tab", id: tab.id, label: tab.named ? tab.label : "" }) });
  rows.push(...cardNoteRows(m, session, shown.masked));
  if (folder) rows.push({ sep: true }, ...folderRows(m, folder, basename(folder), session.icon, session.worktree || session.branch ? session : null, shown));
  if (pr) {
    rows.push({ sep: true }, { label: t("Open {pr}", { pr: pr.label }), icon: "external-link", hint: pr.status ? prWords(pr.status) : undefined, onSelect: () => openUrl(pr.url) });
    if (chat) for (const action of actionsFor(pr)) rows.push({ label: actionLabel(action, pr.label), icon: "send", onSelect: () => void handPR(action, m.id, chat.id, pr.label) });
    if (canRemoveWorktree(session, pr)) rows.push({ label: t("Remove the worktree and its branch…"), icon: "trash", danger: true, onSelect: () => askRemoveWorktree(m, session) });
  }
  const checkout = session.dir ?? folder;
  if (session.branch && checkout && connected) {
    if (!pr) rows.push({ sep: true });
    rows.push({ label: t("Pull requests…"), icon: "git-pull-request", hint: t("its branch's and its repository's: comments, checks, files, merge"), onSelect: () => actions.openPRPanel({ machineId: m.id, dir: checkout, paneId: chat?.id, url: pr && isPullURL(pr.url) ? pr.url : undefined }) });
  }
  rows.push({ sep: true }, { label: t("Previews…"), icon: "external-link", hint: t("what its agents serve, opened in a tab of this browser"), disabled: !connected, onSelect: () => actions.openDialog({ kind: "previews", machineId: m.id, workspace: session.id, name: shown.masked ? shown.name : session.name }) });
  rows.push({ label: t("Triggers…"), icon: "zap", hint: t("what its panes print, and what that sets off"), onSelect: () => actions.openDialog({ kind: "triggers", machineId: m.id, dir: session.dir ?? folder ?? "", workspace: session.id, name: shown.masked ? shown.name : session.name }) });
  const pane = chat ?? tab?.panes[0];
  if (chat || pane) rows.push({ sep: true });
  if (chat && canFork(chat.agent)) rows.push({ label: forkLabel(chat.agent), icon: "copy", disabled: !connected, onSelect: () => void forkPane(m.id, chat.id, chat.agent) });
  if (chat) rows.push({ label: t("Restart {agent} on its session…", { agent: agentTitle(chat.agent) }), icon: "refresh-cw", disabled: !connected, onSelect: () => askRestart(m.id, chat.id, chat.agent) });
  if (tab && pane) rows.push({ label: t("Move the work to another machine…"), icon: "monitor", disabled: !connected, onSelect: () => actions.openDialog({ kind: "teleport", machineId: m.id, paneId: pane.id, tabId: tab.id, workspace: session.name, agent: chat?.agent ?? "" }) });
  rows.push(...sleepRows(m, session));
  if (session.dir) {
    rows.push({ sep: true });
    const dir = session.dir;
    if (muted) rows.push({ label: t("Unmute · {left}", { left: leftWords(muted) }), icon: "bell", onSelect: () => void act(unmuteWorkspace(m.id, dir)) });
    else for (const choice of muteChoices()) rows.push({ label: choice.label, icon: "bell-off", onSelect: () => void act(muteWorkspace(m.id, dir, choice.until)) });
  }
  rows.push({ sep: true });
  if (session.worktree?.linked) rows.push({ label: t("Close & remove worktree…"), icon: "trash", danger: true, disabled: !connected, onSelect: () => actions.openDialog({ kind: "remove-worktree", machineId: m.id, session }) });
  rows.push({ label: t("Close workspace"), icon: "x", danger: true, onSelect: () => closeWorkspace(m, session) });
  return rows;
}
