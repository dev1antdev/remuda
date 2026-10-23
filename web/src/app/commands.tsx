// What the palette offers: every pane to jump to, and every command the page has a button or a menu item
// for — the modes, the lenses, the dialogs, the panels, the rail, the pop-out, the search. The one place
// that lists them; the palette itself only shows and ranks.

import { useCallback } from "react";
import Palette from "@/features/palette/Palette";
import { paneEntries, type Entry } from "@/features/palette/rank";
import { exportConversation } from "@/features/chat/export";
import { DEFAULT_RADAR, rowLabel, type RadarPrefs } from "@/features/roster/radar";
import { closeWorkspace } from "@/features/roster/WorkspaceMenu";
import { moving, useClock, useSpinner } from "@/features/roster/useClocks";
import { useRoster } from "@/features/roster/useRoster";
import { DEFAULT_WALL, tileKey, type WallPrefs } from "@/features/roster/wall";
import { agentTitle, askRestart } from "@/features/updates/restartPane";
import { usePref } from "@/lib/prefs";
import { basename, errMsg } from "@/lib/text";
import type { IconName } from "@/ui/icons";
import { folderOf, useCurrent } from "./current";
import { NEXT_KEY, useNextNeedingMe } from "./next";
import { popOut, popOutSupported } from "./pip";
import { actions, app, type Lens, type Mode } from "./store";

const MODES: { mode: Mode; title: string; hint: string; icon: IconName }[] = [
  { mode: "herdr", title: "Go to Agents", hint: "your agents and terminals, from herdr", icon: "terminal" },
  { mode: "wall", title: "Go to the Wall", hint: "every agent on one screen, a tile each", icon: "layers" },
  { mode: "tasks", title: "Go to Tasks", hint: "your tasks, kept on this remuda", icon: "list" },
  { mode: "code", title: "Go to Code", hint: "VS Code on a machine, through remuda", icon: "code" },
  { mode: "review", title: "Go to Review", hint: "every workspace with changes to review, in one list", icon: "circle-check" },
];

const LENSES: { lens: Lens; title: string; icon: IconName }[] = [
  { lens: "chat", title: "Show the chat", icon: "message" },
  { lens: "terminal", title: "Show the terminal", icon: "terminal" },
  { lens: "changes", title: "Show the changes", icon: "file" },
];

export default function CommandPalette() {
  const entries = useCommands();
  return <Palette entries={entries} onClose={actions.closePanel} />;
}

const toastError = (e: unknown) => actions.notify(errMsg(e), { error: true });

// useCommands is the list for what was typed: the panes first, then the commands that apply right now.
function useCommands(): (query: string) => Entry[] {
  const { machines } = useRoster();
  const mode = app.use((s) => s.mode);
  const lens = app.use((s) => s.lens);
  const viewer = app.use((s) => s.viewer);
  const chatSession = app.use((s) => s.chatSession);
  const footprint = app.use((s) => s.footprint);
  const current = useCurrent();
  const [radar] = usePref<RadarPrefs>("radar", DEFAULT_RADAR);
  const [wall, setWall] = usePref<WallPrefs>("wall", DEFAULT_WALL);
  const [focus, setFocus] = usePref<string>("wall.focus", "");
  const step = useSpinner(moving(machines));
  const now = useClock(machines);
  const goNext = useNextNeedingMe();
  return useCallback(
    (query: string) => {
      const selection = current ? { machineId: current.machine.id, paneId: current.pane.id } : null;
      const panes = paneEntries(machines, radar, now, step, selection, actions.selectPane);
      const commands: Entry[] = [];
      const command = (id: string, title: string, run: () => void, more: Partial<Entry> = {}) => commands.push({ id, kind: "command", title, keywords: [], run, ...more });

      command("next-needs", "Go to the next agent that needs you", goNext, { hint: `${NEXT_KEY} · a question open, a watchdog note, a finish not looked at`, icon: "arrow-right", keywords: ["next", "needs", "attention", "waiting", "blocked", "question", "done", "jump"] });
      for (const m of MODES) if (m.mode !== mode) command(`mode:${m.mode}`, m.title, () => actions.setMode(m.mode), { hint: m.hint, icon: m.icon, keywords: ["mode", m.mode] });
      if (current && !viewer) {
        const folder = folderOf(current);
        for (const l of LENSES) {
          if ((mode === "herdr" && l.lens === lens) || (l.lens === "changes" && !folder)) continue;
          const show = () => {
            actions.setMode("herdr");
            actions.setLens(l.lens);
          };
          command(`lens:${l.lens}`, l.title, show, { hint: `of ${current.session.name}`, icon: l.icon, keywords: ["lens", l.lens, current.pane.agent] });
        }
      }

      const here = current?.machine.id ?? "local";
      for (const m of machines) {
        if (m.state !== "connected" && m.id !== here) continue;
        const where = m.id === here ? "here" : `on ${m.label}`;
        command(`workspace:${m.id}`, `New workspace ${where}…`, () => actions.openDialog({ kind: "workspace", machineId: m.id }), { hint: "an agent in a folder", icon: "plus", keywords: ["start", "agent", "create", m.label] });
      }
      if (current && !viewer) {
        const { machine, session, tab } = current;
        const named = `“${session.name}”`;
        command("tab", `New tab in ${named}…`, () => actions.openDialog({ kind: "tab", machineId: machine.id, session }), { hint: "another agent in the same folder", icon: "plus", keywords: ["agent", "create"] });
        command("worktree", `New worktree of ${named}…`, () => actions.openDialog({ kind: "worktree", machineId: machine.id, session }), { hint: "a branch checked out beside it, an agent in it", icon: "plus", keywords: ["branch", "create"] });
        command("rename-workspace", `Rename workspace ${named}…`, () => actions.openDialog({ kind: "rename", machineId: machine.id, what: "workspace", id: session.id, label: session.name }), { icon: "tag" });
        command("rename-tab", `Rename tab ${tab.named ? `“${tab.label}”` : tab.number}…`, () => actions.openDialog({ kind: "rename", machineId: machine.id, what: "tab", id: tab.id, label: tab.named ? tab.label : "" }), { icon: "tag" });
        if (current.pane.agent) command("restart-pane", `Restart ${agentTitle(current.pane.agent)} on its session…`, () => askRestart(machine.id, current.pane.id, current.pane.agent), { hint: "its own exit, then started again on the same conversation", icon: "refresh-cw", keywords: ["restart", "resume", "stuck", "hung", "reload", "again"] });
        command("close-workspace", `Close workspace ${named}`, () => closeWorkspace(machine, session), { hint: "its panes close and the agents in them stop", icon: "x", danger: true, keywords: ["kill", "stop", "quit"] });
        if (popOutSupported) command("popout", "Pop out this agent", () => void popOut(selection!), { hint: "a small window above every other app", icon: "pip", keywords: ["picture", "window", "float"] });
        if (current.pane.agent) command("export", "Export this conversation as Markdown", () => void exportConversation(machine.id, current.pane.id, chatSession, folderOf(current), { agent: current.pane.agent, workspace: session.name, machine: machine.label }).catch(toastError), { hint: "the session shown, as a file", icon: "download", keywords: ["save", "download", "markdown", "transcript", "share", "file"] });
      }
      if (viewer) {
        const machine = machines.find((m) => m.id === viewer.machine);
        command("export-viewer", "Export this transcript as Markdown", () => void exportConversation(viewer.machine, "", viewer.session, viewer.dir, { agent: "", workspace: basename(viewer.dir) || viewer.dir, machine: machine?.label ?? viewer.machine }).catch(toastError), { hint: "the one open from the search, as a file", icon: "download", keywords: ["save", "download", "markdown", "share", "file"] });
      }

      const screens = wall.view === "screen";
      const onTheWall = (change: Partial<WallPrefs>) => () => {
        setWall({ ...wall, ...change });
        actions.setMode("wall");
      };
      command("wall-needs", wall.needsMe ? "Wall: every agent" : "Wall: only the agents that need me", onTheWall({ needsMe: !wall.needsMe }), { hint: wall.needsMe ? "the filter off" : "a question open, a watchdog note, a finish not looked at", icon: "layers", keywords: ["wall", "needs", "attention", "waiting", "blocked", "filter"] });
      command("wall-idle", wall.hideIdle ? "Wall: show the idle agents" : "Wall: hide the idle agents", onTheWall({ hideIdle: !wall.hideIdle }), { hint: `idle for over ${radar.freshMinutes} min`, icon: "layers", keywords: ["wall", "idle", "hide", "filter"] });
      if (focus) command("wall-unfocus", "Wall: back to the grid", () => {
        setFocus("");
        actions.setMode("wall");
      }, { hint: "every tile its own size again", icon: "minimize", keywords: ["wall", "focus", "grid", "leave"] });
      if (current && current.pane.agent && tileKey(current.machine.id, current.pane.id) !== focus)
        command("wall-focus", `Focus ${rowLabel(current.pane, current.tab, current.session.name, radar)} on the Wall`, () => {
          setFocus(tileKey(current.machine.id, current.pane.id));
          actions.setMode("wall");
        }, { hint: "this agent takes the wall, the others wait in a strip", icon: "maximize", keywords: ["wall", "focus", "big", "zoom"] });
      command("wall-view", screens ? "Wall of chats" : "Wall of screens", onTheWall({ view: screens ? "chat" : "screen" }), { hint: screens ? "each tile the last turns of its conversation" : "each tile its terminal as it is, scaled to fit", icon: "layers", keywords: ["wall", "terminal", "screen", "chat", "view", "tiles"] });
      command("rail", footprint ? "Hide the files rail" : "Show the files rail", actions.toggleFootprint, { hint: "the files the agent touched, beside the lenses", icon: "folder", keywords: ["footprint", "files"] });
      command("activity", "Activity", () => actions.openPanel("activity"), { hint: "what happened across the agents while you were away", icon: "bell", keywords: ["feed", "history", "log", "news", "notifications", "away"] });
      command("broadcast", "Message several agents…", () => actions.openDialog({ kind: "broadcast" }), { hint: "one text to the agents you tick, each in turn", icon: "send", keywords: ["broadcast", "all", "every", "everyone", "prompt", "compact", "send"] });
      command("alerts", "Alerts on this device", () => actions.openPanel("alerts"), { hint: "which pushes to get here", icon: "bell", keywords: ["push", "notifications"] });
      command("schedules", "Schedules", () => actions.openPanel("schedules"), { hint: "prompts sent to agents on a timetable", icon: "calendar", keywords: ["cron", "timer", "prompt"] });
      command("usage", "Usage report", () => actions.openPanel("usage"), { hint: "what the agents cost, by day, project and session", icon: "chart", keywords: ["cost", "tokens", "context", "money"] });
      command("machine", "Add a PC…", () => actions.openDialog({ kind: "machine" }), { hint: "a remote machine over SSH", icon: "monitor", keywords: ["machine", "remote", "ssh", "host"] });
      const typed = query.trim();
      command("search", typed ? `Search every conversation for “${typed}”` : "Search every conversation…", () => actions.openSearch(typed), { hint: "what you or an agent said, on every machine", icon: "search", keywords: ["find", "transcript"], always: true });
      return [...panes, ...commands];
    },
    [machines, mode, lens, viewer, chatSession, footprint, current, radar, wall, setWall, focus, setFocus, step, now, goNext],
  );
}
