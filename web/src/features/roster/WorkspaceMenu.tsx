// The workspace's ⋯ menu: a tab, a worktree, a rename, the pull request the agent named with what its state allows —
// its failed checks or its review handed to the agent, a merged worktree removed — the active tab's agent restarted on
// its session, close.

import { Fragment, useEffect } from "react";
import { act } from "@/app/act";
import { actions, type RowMenu } from "@/app/store";
import { agentTitle, askRestart } from "@/features/updates/restartPane";
import { openUrl } from "@/lib/host";
import { killSession, type Machine, type Session } from "./api";
import { actionLabel, actionsFor, canRemoveWorktree } from "./prActions";
import { askRemoveWorktree, handPR } from "./prMenu";
import { prWords } from "./prStatus";
import { t, tn } from "@/lib/i18n";

export function closeWorkspace(m: Machine, s: Session) {
  const n = s.tabs.reduce((a, t) => a + t.panes.length, 0);
  actions.ask(tn(n, "Close “{name}”? {n} pane closes and the agent in it stops.", "Close “{name}”? {n} panes close and the agents in them stop.", { name: s.name }), t("Close workspace"), () => void act(killSession(m.id, s.id)));
}

export default function WorkspaceMenu({ menu }: { menu: NonNullable<RowMenu> }) {
  useEffect(() => {
    const close = () => actions.closeMenu();
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && close();
    document.addEventListener("click", close);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("click", close);
      document.removeEventListener("keydown", onKey);
    };
  }, []);
  const { machine, session } = menu;
  const tab = session.tabs.find((x) => x.id === session.active_tab_id) ?? session.tabs[0];
  const chat = tab?.panes.find((p) => p.agent);
  const pr = chat?.pr;
  const item = (label: string, run: () => void, className?: string) => (
    <button
      type="button"
      role="menuitem"
      className={className}
      onClick={() => {
        actions.closeMenu();
        run();
      }}
    >
      {label}
    </button>
  );
  return (
    <div className="row-menu" role="menu" style={{ left: menu.x, top: menu.y }} onClick={(e) => e.stopPropagation()}>
      {item("New tab…", () => actions.openDialog({ kind: "tab", machineId: machine.id, session }))}
      {item("New worktree…", () => actions.openDialog({ kind: "worktree", machineId: machine.id, session }))}
      {item("Rename workspace…", () => actions.openDialog({ kind: "rename", machineId: machine.id, what: "workspace", id: session.id, label: session.name }))}
      {tab && item(`Rename tab ${tab.named ? `“${tab.label}”` : tab.number}…`, () => actions.openDialog({ kind: "rename", machineId: machine.id, what: "tab", id: tab.id, label: tab.named ? tab.label : "" }))}
      {pr && item(`Open ${pr.label}${pr.status ? ` · ${prWords(pr.status)}` : ""}`, () => openUrl(pr.url))}
      {pr && chat && actionsFor(pr).map((action) => <Fragment key={action}>{item(actionLabel(action, pr.label), () => void handPR(action, machine.id, chat.id, pr.label))}</Fragment>)}
      {pr && canRemoveWorktree(session, pr) && item(t("Remove the worktree and its branch…"), () => askRemoveWorktree(machine, session), "danger")}
      {chat && item(t("Restart {agent} on its session…", { agent: agentTitle(chat.agent) }), () => askRestart(machine.id, chat.id, chat.agent))}
      {item("Close workspace", () => closeWorkspace(machine, session), "danger")}
    </div>
  );
}
