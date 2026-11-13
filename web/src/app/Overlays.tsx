// Everything that opens over the page: the "make something" dialogs, the confirm, the
// panels (the palette, search, alerts, schedules, usage, the release install) and the file viewer. Each reads what it
// needs from the store and closes itself through the actions.

import { addMachine, createSession, createTab, renameMachine, renameSession, renameTab, type Created } from "@/features/roster/api";
import NewWorktreeDialog from "@/features/roster/worktree/NewWorktreeDialog";
import RemoveWorktreeDialog from "@/features/roster/worktree/RemoveWorktreeDialog";
import { madeWords } from "@/features/roster/worktree/made";
import ProjectSettingsDialog from "@/features/roster/project/ProjectSettingsDialog";
import IconDialog from "@/features/icons/IconDialog";
import LeftoversDialog from "@/features/roster/LeftoversDialog";
import NoteDialog from "@/features/roster/NoteDialog";
import PreviewsDialog from "@/features/preview/PreviewsDialog";
import TriggersDialog from "@/features/triggers/TriggersDialog";
import ServerPanel from "@/features/roster/ServerPanel";
import EverywhereDialog from "@/features/updates/EverywhereDialog";
import FleetDialog from "@/features/updates/FleetDialog";
import ReopenDialog from "@/features/reopen/ReopenDialog";
import ResumeDialog from "@/features/sleep/ResumeDialog";
import ArenaDialog from "@/features/arena/ArenaDialog";
import ArenaPanel from "@/features/arena/ArenaPanel";
import PRPanel from "@/features/pr/PRPanel";
import { useRoster } from "@/features/roster/useRoster";
import { useSharing } from "@/features/roster/useSharing";
import { AddMachineDialog, NewTabDialog, NewWorkspaceDialog, RenameDialog, RenameMachineDialog } from "@/features/roster/dialogs";
import Activity from "@/features/activity/Activity";
import BroadcastDialog from "@/features/agents/BroadcastDialog";
import PinsDialog from "@/features/chat/PinsDialog";
import TimelapseDialog from "@/features/terminal/timelapse/TimelapseDialog";
import UndoDialog from "@/features/undo/UndoDialog";
import KeysDialog from "@/features/help/KeysDialog";
import SettingsDialog from "@/features/settings/SettingsDialog";
import { useAgentTargets } from "@/features/agents/useAgentTargets";
import AlertsPanel from "@/features/alerts/AlertsPanel";
import FileViewer from "@/features/files/FileViewer";
import Schedules from "@/features/schedules/Schedules";
import Search from "@/features/search/Search";
import RecapDialog from "@/features/standup/RecapDialog";
import HandoverDialog from "@/features/chat/HandoverDialog";
import TeleportDialog from "@/features/teleport/TeleportDialog";
import ModelDialog from "@/features/chat/ModelDialog";
import { handedOverWords } from "@/features/chat/handover";
import ImportIssueDialog from "@/features/tasks/ImportIssueDialog";
import IssuesPanel from "@/features/issues/IssuesPanel";
import ImportJiraDialog from "@/features/tasks/ImportJiraDialog";
import RecipesDialog from "@/features/recipes/RecipesDialog";
import StartFromRecipeDialog from "@/features/recipes/StartFromRecipeDialog";
import RunSceneDialog from "@/features/scenes/RunSceneDialog";
import ScenesDialog from "@/features/scenes/ScenesDialog";
import Standup from "@/features/standup/StandupPanel";
import Concierge from "./Concierge";
import ReposPanel from "@/features/repos/ReposPanel";
import UpdatesDialog from "@/features/updates/Updates";
import BudgetDialog from "@/features/usage/BudgetDialog";
import Usage from "@/features/usage/Usage";
import { encodePath, type SearchHit } from "@/features/search/api";
import type { ScreenHit } from "@/features/terminal/timelapse/api";
import { firstStep } from "@/features/terminal/timelapse/found";
import { timelapseTitle } from "@/features/terminal/timelapse/row";
import { machineLabel } from "@/features/roster/options";
import { ConfirmDialog } from "@/ui/ConfirmDialog";
import CommandPalette from "./commands";
import { useAllPanes } from "./current";
import { actions, app } from "./store";
import { t } from "@/lib/i18n";

export default function Overlays() {
  const dialog = app.use((s) => s.dialog);
  const targets = useAgentTargets();
  const panel = app.use((s) => s.panel);
  const search = app.use((s) => s.search);
  const confirm = app.use((s) => s.confirm);
  const file = app.use((s) => s.file);
  const { machines, agents, refresh, host, server, viewOnly } = useRoster();
  const share = useSharing(); // a confirm names what it acts on: under the screen-sharing mode, by its neutral name
  const panes = useAllPanes();
  const leftoversOn = dialog?.kind === "leftovers" ? machines.find((m) => m.id === dialog.machineId) : undefined;
  const previewsOn = dialog?.kind === "previews" ? machines.find((m) => m.id === dialog.machineId) : undefined;
  const worktreeOn = dialog?.kind === "worktree" ? machines.find((m) => m.id === dialog.machineId) : undefined;
  const removeOn = dialog?.kind === "remove-worktree" ? machines.find((m) => m.id === dialog.machineId) : undefined;

  // the dialog closes even on a warning: the container exists although its agent did not start, and a second submit would make another
  const landOn = async (machineId: string, made: Created) => {
    actions.closeDialog();
    refresh();
    actions.selectPane(machineId, made.pane);
    if (made.warning) actions.notify(made.warning);
    if (made.note) actions.notify(made.note);
  };
  const done = async () => {
    actions.closeDialog();
    refresh();
  };
  // a hit's project is the folder the agent started in: a cd inside the session does not move the transcript
  const openHit = (hit: SearchHit) => {
    actions.closePanel();
    const there = panes.filter((x) => x.machine.id === hit.machine && (encodePath(x.session.dir ?? "") === hit.project || encodePath(x.pane.cwd) === hit.project));
    const pick = there.find((x) => x.pane.agent === "claude") ?? there.find((x) => x.pane.agent); // a shell has no chat to jump in: the transcript opens on its own
    actions.jumpTo(hit.machine, pick ? pick.pane.id : "", hit.session, hit.uuid);
    if (pick) actions.selectPane(pick.machine.id, pick.pane.id);
    else actions.openViewer(hit);
  };
  // a screen's hit opens the player on the recording's pane and folder, at the frame the text first stood in
  const openScreenHit = (hit: ScreenHit, query: string) => {
    actions.closePanel();
    const title = timelapseTitle({ machineId: hit.machine, workspace: hit.workspace, machine: machineLabel(machines, hit.machine) });
    actions.openDialog({ kind: "timelapse", machineId: hit.machine, paneId: hit.pane, dir: hit.dir, title, found: { ...firstStep(hit), query } });
  };

  return (
    <>
      {panel === "palette" && <CommandPalette />}
      {panel === "release" && host && <UpdatesDialog machine={host} what="remuda-release" onClose={actions.closePanel} />}
      {panel === "server" && server && <ServerPanel server={server} onClose={actions.closePanel} />}
      {dialog?.kind === "update-everywhere" && <EverywhereDialog onClose={actions.closeDialog} />}
      {dialog?.kind === "fleet-setup" && <FleetDialog onClose={actions.closeDialog} />}
      {panel === "alerts" && <AlertsPanel onClose={actions.closePanel} />}
      {panel === "activity" && <Activity onClose={actions.closePanel} />}
      {panel === "schedules" && <Schedules machines={machines} onClose={actions.closePanel} ask={actions.ask} />}
      {panel === "search" && <Search machines={machines} initial={search} onClose={actions.closePanel} onOpen={openHit} onOpenScreen={openScreenHit} />}
      {panel === "standup" && <Standup onClose={actions.closePanel} />}
      {panel === "arena" && <ArenaPanel onClose={actions.closePanel} />}
      {panel === "concierge" && !viewOnly && <Concierge onClose={actions.closePanel} />}
      {panel === "pr" && !viewOnly && <PRPanel onClose={actions.closePanel} />}
      {panel === "issues" && !viewOnly && <IssuesPanel onClose={actions.closePanel} />}
      {panel === "repos" && !viewOnly && <ReposPanel onClose={actions.closePanel} />}
      {panel === "usage" && (
        <Usage machines={machines} onClose={actions.closePanel} onOpenSession={(machine, project, session, dir) => openHit({ machine, project, session, dir, uuid: "", ts: "", role: "assistant", snippet: "" })} />
      )}
      {file && <FileViewer machineId={file.machineId} paneId={file.paneId} dir={file.dir} target={file.target} onClose={actions.closeFile} onRetarget={actions.retargetFile} />}
      {confirm && (
        <ConfirmDialog
          text={share.on ? share.redact(confirm.text) : confirm.text}
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
      {dialog?.kind === "folder-icon" && <IconDialog machineId={dialog.machineId} folder={dialog.folder} name={dialog.name} icon={dialog.icon} onClose={actions.closeDialog} />}
      {dialog?.kind === "note" && !viewOnly && <NoteDialog machineId={dialog.machineId} dir={dialog.dir} name={dialog.name} text={dialog.text} onClose={actions.closeDialog} />}
      {leftoversOn && <LeftoversDialog machine={leftoversOn} onClose={actions.closeDialog} />}
      {dialog?.kind === "triggers" && <TriggersDialog machineId={dialog.machineId} dir={dialog.dir} workspace={dialog.workspace} name={dialog.name} onClose={actions.closeDialog} />}
      {previewsOn && dialog?.kind === "previews" && (
        <PreviewsDialog machineId={previewsOn.id} machineLabel={previewsOn.label} workspace={dialog.workspace} workspaceName={dialog.name} onClose={actions.closeDialog} />
      )}
      {dialog?.kind === "reopen" && <ReopenDialog onClose={actions.closeDialog} />}
      {dialog?.kind === "resume-after-restart" && !viewOnly && <ResumeDialog machineId={dialog.machineId} onClose={actions.closeDialog} />}
      {dialog?.kind === "arena" && <ArenaDialog taskId={dialog.taskId} title={dialog.title} prompt={dialog.prompt} at={dialog.at} lanes={dialog.lanes} onClose={actions.closeDialog} />}
      {dialog?.kind === "workspace" && (
        <NewWorkspaceDialog
          machines={machines}
          agents={agents}
          defaultMachine={dialog.machineId}
          defaultDir={dialog.dir}
          onClose={actions.closeDialog}
          onCreate={async (machine, dir, name, agent, extras) => landOn(machine, await createSession({ machine, name, dir, agent, ...extras }))}
        />
      )}
      {dialog?.kind === "tab" && (
        <NewTabDialog
          session={dialog.session}
          machine={dialog.machineId}
          agents={agents}
          onClose={actions.closeDialog}
          onCreate={async (name, agent, extras) => landOn(dialog.machineId, await createTab({ machine: dialog.machineId, workspace: dialog.session.id, name, agent, ...extras }))}
        />
      )}
      {dialog?.kind === "worktree" && worktreeOn && (
        <NewWorktreeDialog
          machine={worktreeOn}
          session={dialog.session}
          agents={agents}
          issue={dialog.issue}
          repo={dialog.repo}
          title={dialog.title}
          prompt={dialog.prompt}
          onClose={actions.closeDialog}
          onMade={(made) => {
            actions.closeDialog();
            refresh();
            if (made.pane) actions.selectPane(worktreeOn.id, made.pane);
            for (const text of madeWords(made)) actions.notify(text, { error: !!made.warning && text === made.warning });
          }}
        />
      )}
      {dialog?.kind === "remove-worktree" && removeOn && (
        <RemoveWorktreeDialog
          machine={removeOn}
          session={dialog.session}
          onClose={actions.closeDialog}
          onDone={() => {
            actions.closeDialog();
            refresh();
          }}
        />
      )}
      {dialog?.kind === "project-settings" && <ProjectSettingsDialog machineId={dialog.machineId} root={dialog.root} onClose={actions.closeDialog} />}
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
      {dialog?.kind === "import-issue" && (
        <ImportIssueDialog
          projectId={dialog.projectId}
          url={dialog.url}
          machineId={dialog.machineId}
          onClose={actions.closeDialog}
          onCreated={(task, start) => {
            actions.closeDialog();
            actions.openTask(task.project_id, task.id, start);
          }}
        />
      )}
      {dialog?.kind === "import-jira" && (
        <ImportJiraDialog
          projectId={dialog.projectId}
          onClose={actions.closeDialog}
          onCreated={(task, start) => {
            actions.closeDialog();
            actions.openTask(task.project_id, task.id, start);
          }}
        />
      )}
      {dialog?.kind === "recap" && <RecapDialog machineId={dialog.machineId} paneId={dialog.paneId} session={dialog.session} dir={dialog.dir} title={dialog.title} onClose={actions.closeDialog} />}
      {dialog?.kind === "handover" && (
        <HandoverDialog
          machineId={dialog.machineId}
          paneId={dialog.paneId}
          session={dialog.session}
          agent={dialog.agent}
          workspace={dialog.workspace}
          agents={agents}
          onClose={actions.closeDialog}
          onDone={async (kind, made) => {
            actions.closeDialog();
            refresh();
            actions.selectPane(dialog.machineId, made.pane);
            actions.setLens("chat");
            const label = agents.find((a) => a.id === kind)?.label ?? kind;
            if (made.agent_error) actions.notify(t("The tab is there, but {agent} failed to start: {error}", { agent: label, error: made.agent_error }), { error: true });
            else actions.notify(handedOverWords(label, made.size), { brief: true });
          }}
        />
      )}
      {dialog?.kind === "teleport" && (
        <TeleportDialog machineId={dialog.machineId} paneId={dialog.paneId} tabId={dialog.tabId} workspace={dialog.workspace} agent={dialog.agent} agents={agents} onClose={actions.closeDialog} />
      )}
      {dialog?.kind === "model" && (
        <ModelDialog
          machineId={dialog.machineId}
          paneId={dialog.paneId}
          current={dialog.current}
          onClose={actions.closeDialog}
          onDone={(id) => {
            actions.closeDialog();
            actions.notify(t("Told Claude Code /model {model}", { model: id }), { brief: true });
          }}
        />
      )}
      {dialog?.kind === "budget" && <BudgetDialog machines={machines} budget={dialog.budget} onClose={actions.closeDialog} onSaved={done} ask={actions.ask} />}
      {dialog?.kind === "recipes" && <RecipesDialog onClose={actions.closeDialog} />}
      {dialog?.kind === "scenes" && <ScenesDialog onClose={actions.closeDialog} onRun={(id) => actions.openDialog({ kind: "scene-run", id })} />}
      {dialog?.kind === "scene-run" && <RunSceneDialog id={dialog.id} onClose={actions.closeDialog} />}
      {dialog?.kind === "start-recipe" && (
        <StartFromRecipeDialog machineId={dialog.machineId} dir={dialog.dir} onClose={actions.closeDialog} onStart={async (req) => landOn(req.machine, await createSession(req))} />
      )}
      {dialog?.kind === "pins" && <PinsDialog machineId={dialog.machineId} paneId={dialog.paneId} dir={dialog.dir} shown={dialog.shown} title={dialog.title} onClose={actions.closeDialog} />}
      {dialog?.kind === "timelapse" && (
        <TimelapseDialog
          key={dialog.found ? `${dialog.found.recording}@${dialog.found.frame}` : `${dialog.machineId}|${dialog.paneId}`}
          machineId={dialog.machineId}
          paneId={dialog.paneId}
          dir={dialog.dir}
          title={dialog.title}
          found={dialog.found}
          onClose={actions.closeDialog}
        />
      )}
      {dialog?.kind === "undo-turn" && <UndoDialog machineId={dialog.machineId} dir={dialog.dir} turn={dialog.turn} onClose={actions.closeDialog} />}
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
