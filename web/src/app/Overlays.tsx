// Everything that opens over the page: the "make something" dialogs, the workspace menu, the confirm, the
// panels (the palette, search, alerts, schedules, usage, the release install) and the file viewer. Each reads what it
// needs from the store and closes itself through the actions.

import { addMachine, createSession, createTab, createWorktree, renameMachine, renameSession, renameTab, type Created } from "@/features/roster/api";
import { useRoster } from "@/features/roster/useRoster";
import { AddMachineDialog, NewTabDialog, NewWorkspaceDialog, NewWorktreeDialog, RenameDialog, RenameMachineDialog } from "@/features/roster/dialogs";
import WorkspaceMenu from "@/features/roster/WorkspaceMenu";
import Activity from "@/features/activity/Activity";
import BroadcastDialog from "@/features/agents/BroadcastDialog";
import KeysDialog from "@/features/help/KeysDialog";
import SettingsDialog from "@/features/settings/SettingsDialog";
import { useAgentTargets } from "@/features/agents/useAgentTargets";
import AlertsPanel from "@/features/alerts/AlertsPanel";
import FileViewer from "@/features/files/FileViewer";
import Schedules from "@/features/schedules/Schedules";
import Search from "@/features/search/Search";
import RecapDialog from "@/features/standup/RecapDialog";
import Standup from "@/features/standup/StandupPanel";
import UpdatesDialog from "@/features/updates/Updates";
import BudgetDialog from "@/features/usage/BudgetDialog";
import Usage from "@/features/usage/Usage";
import { encodePath, type SearchHit } from "@/features/search/api";
import { ConfirmDialog } from "@/ui/ConfirmDialog";
import CommandPalette from "./commands";
import { useAllPanes } from "./current";
import { actions, app } from "./store";

export default function Overlays() {
  const dialog = app.use((s) => s.dialog);
  const targets = useAgentTargets();
  const menu = app.use((s) => s.menu);
  const panel = app.use((s) => s.panel);
  const search = app.use((s) => s.search);
  const confirm = app.use((s) => s.confirm);
  const file = app.use((s) => s.file);
  const { machines, agents, refresh } = useRoster();
  const panes = useAllPanes();
  const local = machines.find((m) => m.id === "local");

  // the dialog closes even on a warning: the container exists although its agent did not start, and a second submit would make another
  const landOn = async (machineId: string, made: Created) => {
    actions.closeDialog();
    refresh();
    actions.selectPane(machineId, made.pane);
    if (made.warning) actions.notify(made.warning);
  };
  const done = async () => {
    actions.closeDialog();
    refresh();
  };
  // a hit's project is the folder the agent started in: a cd inside the session does not move the transcript
  const openHit = (hit: SearchHit) => {
    actions.closePanel();
    const there = panes.filter((x) => x.machine.id === hit.machine && (encodePath(x.session.dir ?? "") === hit.project || encodePath(x.pane.cwd) === hit.project));
    const pick = there.find((x) => x.pane.agent === "claude") ?? there.find((x) => x.pane.agent) ?? there[0];
    actions.jumpTo(hit.machine, pick ? pick.pane.id : "", hit.session, hit.uuid);
    if (pick) actions.selectPane(pick.machine.id, pick.pane.id);
    else actions.openViewer(hit);
  };

  return (
    <>
      {panel === "palette" && <CommandPalette />}
      {panel === "release" && local && <UpdatesDialog machine={local} what="remuda-release" onClose={actions.closePanel} />}
      {panel === "alerts" && <AlertsPanel onClose={actions.closePanel} />}
      {panel === "activity" && <Activity onClose={actions.closePanel} />}
      {panel === "schedules" && <Schedules machines={machines} onClose={actions.closePanel} ask={actions.ask} />}
      {panel === "search" && <Search machines={machines} initial={search} onClose={actions.closePanel} onOpen={openHit} />}
      {panel === "standup" && <Standup onClose={actions.closePanel} />}
      {panel === "usage" && (
        <Usage machines={machines} onClose={actions.closePanel} onOpenSession={(machine, project, session, dir) => openHit({ machine, project, session, dir, uuid: "", ts: "", role: "assistant", snippet: "" })} />
      )}
      {file && <FileViewer machineId={file.machineId} paneId={file.paneId} dir={file.dir} target={file.target} onClose={actions.closeFile} onRetarget={actions.retargetFile} />}
      {menu && <WorkspaceMenu menu={menu} />}
      {confirm && (
        <ConfirmDialog
          text={confirm.text}
          label={confirm.label}
          onClose={actions.closeConfirm}
          onConfirm={() => {
            actions.closeConfirm();
            confirm.run();
          }}
        />
      )}
      {dialog?.kind === "rename" && (
        <RenameDialog
          what={dialog.what}
          label={dialog.label}
          onClose={actions.closeDialog}
          onRename={async (label) => {
            await (dialog.what === "tab" ? renameTab : renameSession)(dialog.machineId, dialog.id, label);
            await done();
          }}
        />
      )}
      {dialog?.kind === "workspace" && (
        <NewWorkspaceDialog
          machines={machines}
          agents={agents}
          defaultMachine={dialog.machineId}
          onClose={actions.closeDialog}
          onCreate={async (machine, dir, name, agent) => landOn(machine, await createSession({ machine, name, dir, agent }))}
        />
      )}
      {dialog?.kind === "tab" && (
        <NewTabDialog
          session={dialog.session}
          agents={agents}
          onClose={actions.closeDialog}
          onCreate={async (name, agent) => landOn(dialog.machineId, await createTab({ machine: dialog.machineId, workspace: dialog.session.id, name, agent }))}
        />
      )}
      {dialog?.kind === "worktree" && (
        <NewWorktreeDialog
          session={dialog.session}
          agents={agents}
          onClose={actions.closeDialog}
          onCreate={async (branch, base, name, agent) =>
            landOn(dialog.machineId, await createWorktree({ machine: dialog.machineId, workspace: dialog.session.id, branch, base, name, agent }))
          }
        />
      )}
      {dialog?.kind === "renameMachine" && (
        <RenameMachineDialog
          machine={dialog.machine}
          onClose={actions.closeDialog}
          onRename={async (label) => {
            await renameMachine(dialog.machine.id, label);
            await done();
          }}
        />
      )}
      {dialog?.kind === "broadcast" && <BroadcastDialog agents={targets} onClose={actions.closeDialog} />}
      {dialog?.kind === "keys" && <KeysDialog onClose={actions.closeDialog} />}
      {dialog?.kind === "settings" && <SettingsDialog onClose={actions.closeDialog} />}
      {dialog?.kind === "recap" && <RecapDialog machineId={dialog.machineId} paneId={dialog.paneId} session={dialog.session} dir={dialog.dir} title={dialog.title} onClose={actions.closeDialog} />}
      {dialog?.kind === "budget" && <BudgetDialog machines={machines} budget={dialog.budget} onClose={actions.closeDialog} onSaved={done} ask={actions.ask} />}
      {dialog?.kind === "machine" && (
        <AddMachineDialog
          onClose={actions.closeDialog}
          onAdd={async (label, host) => {
            await addMachine({ label, host });
            await done();
          }}
        />
      )}
    </>
  );
}
