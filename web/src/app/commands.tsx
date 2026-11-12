// What the palette offers: every pane to jump to, and every command the page has a button or a menu item
// for — the modes, the lenses, the dialogs, the panels, the rail, the pop-out, the search. The one place
// that lists them; the palette itself only shows and ranks.

import { useCallback } from "react";
import Palette from "@/features/palette/Palette";
import { paneEntries, paneTitle, tabName, type Entry } from "@/features/palette/rank";
import { exportConversation } from "@/features/chat/export";
import { useTimelineShown } from "@/features/chat/TimelineBar";
import { DEFAULT_RADAR, type RadarPrefs } from "@/features/roster/radar";
import { closeWorkspace } from "@/features/roster/WorkspaceMenu";
import { moving, useClock, useSpinner } from "@/features/roster/useClocks";
import { useRoster } from "@/features/roster/useRoster";
import { DEFAULT_WALL, tileKey, type WallPrefs } from "@/features/roster/wall";
import { agentTitle, askRestart } from "@/features/updates/restartPane";
import { isMac, usePref, writePref } from "@/lib/prefs";
import { DEFAULT_OPEN_LENS, OPEN_LENS_KEY, openLensOf, type OpenLens } from "./openLens";
import { lensOn, lensesOf } from "./paneLens";
import { basename, errMsg } from "@/lib/text";
import { folderOf as sessionFolder } from "@/features/roster/workspaces";
import { canFork, forkLabel } from "@/features/chat/fork";
import { forkPane } from "@/features/chat/forkPane";
import { keyWords } from "@/features/terminal/float/float";
import { toggleFloat } from "@/features/terminal/float/useFloatKey";
import { openTimelapse } from "@/features/terminal/timelapse/row";
import { setSharingAll, toggleSharing, toggleSharingFolder, toggleSharingMachine, useSharing } from "@/features/roster/useSharing";
import { markedFolder, markedMachine } from "@/features/roster/sharing";
import { SHARING_KEY_NAME } from "./sharingKey";
import { resetPanels } from "@/ui";
import type { IconName } from "@/ui/icons";
import { folderOf, useCurrent } from "./current";
import { enterFull, leaveFull } from "./full";
import { NEXT_KEY, useNextNeedingMe } from "./next";
import { REOPEN_KEY } from "@/features/reopen/closed";
import { USAGE_TAB_KEY, type UsageTab } from "@/features/usage/scorecard";
import { popOut, popOutSupported } from "./pip";
import { actions, app, type Lens, type Mode } from "./store";
import { setDarkFlavorPref, setTextSizePref, setThemePref, useDarkFlavorPref, useTextSize, useThemePref } from "./theme";
import { TEXT_SIZE_NAMES, TEXT_SIZES } from "@/lib/look";
import { currentLang, english, LANGS, t } from "@/lib/i18n";
import { setLangPref } from "./lang";
import { viewerCommand } from "./viewOnly";
import { DARK_FLAVORS, FLAVOR_NAMES } from "@/lib/theme";

const FIND_KEY = isMac ? "⌘F" : "Ctrl+F";

const MODES: { mode: Mode; title: string; hint: string; icon: IconName }[] = [
  { mode: "herdr", title: t("Go to Agents"), hint: t("your agents and terminals, from herdr"), icon: "terminal" },
  { mode: "wall", title: t("Go to the Wall"), hint: t("every agent on one screen, a tile each"), icon: "layers" },
  { mode: "tasks", title: t("Go to Tasks"), hint: t("your tasks, kept on this remuda"), icon: "list" },
  { mode: "code", title: t("Go to Code"), hint: t("VS Code on a machine, through remuda"), icon: "code" },
  { mode: "review", title: t("Go to Review"), hint: t("every workspace with changes to review, in one list"), icon: "circle-check" },
];

const LENSES: { lens: Lens; title: string; icon: IconName }[] = [
  { lens: "chat", title: t("Show the chat"), icon: "message" },
  { lens: "terminal", title: t("Show the terminal"), icon: "terminal" },
  { lens: "changes", title: t("Show the changes"), icon: "file" },
];

export default function CommandPalette() {
  const entries = useCommands();
  return <Palette entries={entries} onClose={actions.closePanel} />;
}

const toastError = (e: unknown) => actions.notify(errMsg(e), { error: true });

// useCommands is the list for what was typed: the panes first, then the commands that apply right now.
function useCommands(): (query: string) => Entry[] {
  const { machines, viewOnly, jiraSite, server } = useRoster();
  const mode = app.use((s) => s.mode);
  const viewer = app.use((s) => s.viewer);
  const chatSession = app.use((s) => s.chatSession);
  const chatLive = app.use((s) => s.chatLive);
  const footprint = app.use((s) => s.footprint);
  const split = app.use((s) => s.split);
  const full = app.use((s) => s.full);
  const float = app.use((s) => !!s.float);
  const themePref = useThemePref();
  const openLens = openLensOf(usePref<OpenLens>(OPEN_LENS_KEY, DEFAULT_OPEN_LENS)[0]);
  const darkFlavor = useDarkFlavorPref();
  const textSize = useTextSize();
  const current = useCurrent();
  const lens = lensOn(app.use((s) => s.lens), current?.pane.agent ?? ""); // a shell has its terminal alone
  const [radar] = usePref<RadarPrefs>("radar", DEFAULT_RADAR);
  const [wall, setWall] = usePref<WallPrefs>("wall", DEFAULT_WALL);
  const [focus, setFocus] = usePref<string>("wall.focus", "");
  const [office, setOffice] = usePref<boolean>("wall.office", false);
  const [timelineShown, setTimelineShown] = useTimelineShown();
  const step = useSpinner(moving(machines));
  const now = useClock(machines);
  const goNext = useNextNeedingMe();
  const share = useSharing();
  return useCallback(
    (query: string) => {
      const selection = current ? { machineId: current.machine.id, paneId: current.pane.id } : null;
      const masked = (machine: (typeof machines)[number], session: Parameters<typeof share.sessionHidden>[1]) =>
        share.sessionHidden(machine.id, session) ? { folder: share.folderName(machine.id, sessionFolder(session), session.name), machine: share.machineLabel(machine) } : null;
      const panes = paneEntries(machines, radar, now, step, selection, actions.selectPane, masked);
      const commands: Entry[] = [];
      const command = (id: string, title: string, run: () => void, more: Partial<Entry> = {}) => {
        const original = english(title);
        commands.push({ id, kind: "command", title, run, ...more, keywords: [...(more.keywords ?? []), ...(original === title ? [] : [original])] });
      };

      command("next-needs", t("Go to the next agent that needs you"), goNext, { hint: `${NEXT_KEY} · ${t("a question open, a watchdog note, a finish not looked at")}`, icon: "arrow-right", keywords: ["next", "needs", "attention", "waiting", "blocked", "question", "done", "jump"] });
      for (const m of MODES) if (m.mode !== mode) command(`mode:${m.mode}`, m.title, () => actions.setMode(m.mode), { hint: m.hint, icon: m.icon, keywords: ["mode", m.mode] });
      command("update-everywhere", t("Update everywhere…"), () => actions.openDialog({ kind: "update-everywhere" }), { hint: t("every connected machine's pending updates, one after another"), icon: "refresh-cw", keywords: ["update", "upgrade", "all", "machines", "herdr", "claude", "codex", "release"] });
      if (server) command("server", t("Server · {name}", { name: server.label }), () => actions.togglePanel("server"), { hint: t("its load, memory, temperature and throttling"), icon: "server", keywords: ["server", "host", "pi", "raspberry", "vitals", "temperature", "memory", "throttle"] });
      command("reopen", t("Reopen a closed agent…"), () => actions.openDialog({ kind: "reopen" }), { hint: `${REOPEN_KEY} · ${t("an agent whose tab or workspace closed, back on its own session")}`, icon: "refresh-cw", keywords: ["reopen", "undo", "closed", "restore", "tab", "back", "resume"] });
      if (current && !viewer) {
        const folder = folderOf(current);
        for (const l of LENSES) {
          if ((mode === "herdr" && l.lens === lens) || (l.lens === "changes" && !folder) || !lensesOf(current.pane.agent).includes(l.lens)) continue;
          const show = () => {
            actions.setMode("herdr");
            actions.setLens(l.lens);
          };
          command(`lens:${l.lens}`, l.title, show, { hint: t("of {workspace}", { workspace: current.session.name }), icon: l.icon, keywords: ["lens", l.lens, current.pane.agent] });
        }
      }

      const here = current?.machine.id ?? "local";
      for (const m of machines) {
        if (m.state !== "connected" && m.id !== here) continue;
        const where = m.id === here ? t("New workspace here…") : t("New workspace on {machine}…", { machine: m.label });
        command(`workspace:${m.id}`, where, () => actions.openDialog({ kind: "workspace", machineId: m.id }), { hint: t("an agent in a folder"), icon: "plus", keywords: ["start", "agent", "create", m.label] });
      }
      for (const m of machines.filter((m) => m.state === "connected"))
        command(`previews:${m.id}`, t("Previews on {machine}…", { machine: m.label }), () => actions.openDialog({ kind: "previews", machineId: m.id }), { hint: t("what the agents serve there, opened in a tab of this browser"), icon: "external-link", keywords: ["preview", "port", "dev", "server", "localhost", "open", "browser", m.label] });
      for (const m of machines.filter((m) => m.state === "connected"))
        command(`leftovers:${m.id}`, t("Leftover worktrees on {machine}…", { machine: m.label }), () => actions.openDialog({ kind: "leftovers", machineId: m.id }), { hint: t("worktrees no workspace has open"), icon: "git-branch", keywords: ["cleanup", "clean", "worktree", "garbage", "disk", m.label] });
      if (current && !viewer) {
        const { machine, session, tab } = current;
        const named = `“${session.name}”`;
        const titled = machine.id !== "local" ? t("{workspace} on {machine}", { workspace: session.name, machine: machine.label }) : session.name;
        command("tab", t("New tab in {workspace}…", { workspace: named }), () => actions.openDialog({ kind: "tab", machineId: machine.id, session }), { hint: t("another agent in the same folder"), icon: "plus", keywords: ["agent", "create"] });
        if (session.worktree || session.branch)
          command("worktree", t("New workspace in {workspace}…", { workspace: named }), () => actions.openDialog({ kind: "worktree", machineId: machine.id, session }), { hint: t("a worktree of its own: a new folder and branch, an agent in it"), icon: "git-branch", keywords: ["worktree", "branch", "create", "agent"] });
        command("previews", t("Previews of {workspace}…", { workspace: named }), () => actions.openDialog({ kind: "previews", machineId: machine.id, workspace: session.id, name: session.name }), { hint: t("what its agents serve, opened in a tab of this browser"), icon: "external-link", keywords: ["preview", "port", "dev", "server", "localhost", "open", "browser"] });
        command("rename-workspace", t("Rename workspace {workspace}…", { workspace: named }), () => actions.openDialog({ kind: "rename", machineId: machine.id, what: "workspace", id: session.id, label: session.name }), { icon: "tag" });
        const folder = sessionFolder(session);
        if (folder) command("folder-icon", t("Icon for the folder {folder}…", { folder: `“${basename(folder)}”` }), () => actions.openDialog({ kind: "folder-icon", machineId: machine.id, folder, name: basename(folder), icon: session.icon }), { hint: t("an emoji, a site's icon or a picture"), icon: "folder", keywords: ["emoji", "favicon", "logo", "picture"] });
        command("rename-tab", t("Rename tab {tab}…", { tab: tabName(tab, share.sessionHidden(machine.id, session)) }), () => actions.openDialog({ kind: "rename", machineId: machine.id, what: "tab", id: tab.id, label: tab.named ? tab.label : "" }), { icon: "tag" });
        if (current.pane.agent) command("restart-pane", t("Restart {agent} on its session…", { agent: agentTitle(current.pane.agent) }), () => askRestart(machine.id, current.pane.id, current.pane.agent), { hint: t("its own exit, then started again on the same conversation"), icon: "refresh-cw", keywords: ["restart", "resume", "stuck", "hung", "reload", "again"] });
        command("close-workspace", t("Close workspace {workspace}", { workspace: named }), () => closeWorkspace(machine, session), { hint: t("its panes close and the agents in them stop"), icon: "x", danger: true, keywords: ["kill", "stop", "quit"] });
        if (popOutSupported) command("popout", t("Pop out this agent"), () => void popOut(selection!), { hint: t("a small window above every other app"), icon: "pip", keywords: ["picture", "window", "float"] });
        if (current.pane.agent) command("export", t("Export this conversation as Markdown"), () => void exportConversation(machine.id, current.pane.id, chatSession, folderOf(current), { agent: current.pane.agent, workspace: session.name, machine: machine.label }).catch(toastError), { hint: t("the session shown, as a file"), icon: "download", keywords: ["save", "download", "markdown", "transcript", "share", "file"] });
        if (current.pane.agent) command("recap", t("Recap this conversation"), () => actions.openDialog({ kind: "recap", machineId: machine.id, paneId: current.pane.id, session: chatSession, dir: folderOf(current), title: titled }), { hint: t("prompts, answers, tools, files, cost — Copy as Markdown"), icon: "sun", keywords: ["recap", "summary", "report", "standup", "digest", "what happened"] });
        if (current.pane.agent) command("timelapse", t("Timelapse of this pane…"), () => openTimelapse({ machineId: machine.id, paneId: current.pane.id, dir: folderOf(current), agent: current.pane.agent, workspace: session.name, machine: machine.label }), { hint: t("its turns as they looked on screen, played back in seconds; export as GIF"), icon: "film", keywords: ["timelapse", "replay", "recording", "record", "screen", "gif", "video", "playback", "fast"] });
        if (canFork(current.pane.agent)) command("fork", forkLabel(current.pane.agent), () => void forkPane(machine.id, current.pane.id, current.pane.agent, chatSession), { hint: t("the same history in a new tab, a second path; this one goes on as it is"), icon: "columns", keywords: ["fork", "copy", "branch", "try", "another way", "duplicate", "conversation"] });
        if (current.pane.agent) command("handover", t("Continue this conversation with another agent"), () => actions.openDialog({ kind: "handover", machineId: machine.id, paneId: current.pane.id, session: chatSession, agent: current.pane.agent, workspace: titled }), { hint: t("a new tab here with Codex, Gemini or a fresh Claude, the gist of this conversation as its first prompt"), icon: "arrow-right", keywords: ["handover", "hand over", "continue", "switch", "codex", "gemini", "kimi", "another agent", "model", "cheaper", "stuck"] });
        command("teleport", t("Move the work in {workspace} to another machine…", { workspace: named }), () => actions.openDialog({ kind: "teleport", machineId: machine.id, paneId: current.pane.id, tabId: tab.id, workspace: session.name, agent: current.pane.agent }), { hint: t("its branch, commits and uncommitted changes to a clone there, an agent carrying on"), icon: "send", keywords: ["teleport", "move", "machine", "transfer", "worktree", "laptop"] });
        if (current.pane.agent) command("pins", t("Pins in this conversation…"), () => actions.openDialog({ kind: "pins", machineId: machine.id, paneId: current.pane.id, dir: folderOf(current), shown: chatSession ?? chatLive, title: titled }), { hint: t("the turns you pinned in this folder's conversations, a click opens each"), icon: "pin", keywords: ["pin", "pins", "pinned", "bookmark", "bookmarks", "saved", "mark"] });
        if (current.pane.agent) command("find", t("Find in this conversation"), () => {
          actions.setMode("herdr");
          actions.setLens("chat");
          actions.openFind();
        }, { hint: `${FIND_KEY} ${t("on the chat lens: every match marked, Enter walks them")}`, icon: "search", keywords: ["search", "text", "match", "highlight", "where"] });
        if (current.pane.agent) command("timeline", timelineShown ? t("Hide the session timeline") : t("Show the session timeline"), () => {
          actions.setMode("herdr");
          actions.setLens("chat");
          setTimelineShown(!timelineShown);
        }, { hint: t("prompts and answers in time over the chat; the first error, the longest call, loops and questions marked"), icon: "clock", keywords: ["timeline", "history", "highlights", "errors", "loop", "overview", "map", "time"] });
      }
      if (viewer) {
        const machine = machines.find((m) => m.id === viewer.machine);
        if (!viewer.subagent) command("export-viewer", t("Export this transcript as Markdown"), () => void exportConversation(viewer.machine, "", viewer.session, viewer.dir, { agent: "", workspace: basename(viewer.dir) || viewer.dir, machine: machine?.label ?? viewer.machine }).catch(toastError), { hint: t("the one open from the search, as a file"), icon: "download", keywords: ["save", "download", "markdown", "share", "file"] });
        command("find-viewer", t("Find in this transcript"), () => actions.openFind(), { hint: `${FIND_KEY}: ${t("every match marked, Enter walks them")}`, icon: "search", keywords: ["search", "text", "match", "highlight", "where"] });
        command("recap-viewer", t("Recap this transcript"), () => actions.openDialog({ kind: "recap", machineId: viewer.machine, paneId: "", session: viewer.session, dir: viewer.dir, title: basename(viewer.dir) || viewer.dir }), { hint: t("prompts, answers, tools, files, cost — Copy as Markdown"), icon: "sun", keywords: ["recap", "summary", "report", "standup", "digest"] });
        command("timeline-viewer", timelineShown ? t("Hide the session timeline") : t("Show the session timeline"), () => setTimelineShown(!timelineShown), { hint: t("prompts and answers in time over the transcript; the first error, the longest call, loops and questions marked"), icon: "clock", keywords: ["timeline", "history", "highlights", "errors", "loop", "overview", "map", "time"] });
      }

      const screens = wall.view === "screen";
      const onTheWall = (change: Partial<WallPrefs>) => () => {
        setWall({ ...wall, ...change });
        actions.setMode("wall");
      };
      command("wall-needs", wall.needsMe ? t("Wall: every agent") : t("Wall: only the agents that need me"), onTheWall({ needsMe: !wall.needsMe }), { hint: wall.needsMe ? t("the filter off") : t("a question open, a watchdog note, a finish not looked at"), icon: "layers", keywords: ["wall", "needs", "attention", "waiting", "blocked", "filter"] });
      command("wall-idle", wall.hideIdle ? t("Wall: show the idle agents") : t("Wall: hide the idle agents"), onTheWall({ hideIdle: !wall.hideIdle }), { hint: t("idle for over {n} min", { n: radar.freshMinutes }), icon: "layers", keywords: ["wall", "idle", "hide", "filter"] });
      if (focus) command("wall-unfocus", t("Wall: back to the grid"), () => {
        setFocus("");
        actions.setMode("wall");
      }, { hint: t("every tile its own size again"), icon: "minimize", keywords: ["wall", "focus", "grid", "leave"] });
      if (current && current.pane.agent && tileKey(current.machine.id, current.pane.id) !== focus)
        command("wall-focus", t("Focus {agent} on the Wall", { agent: paneTitle(current.pane, current.tab, current.session.name, radar, share.sessionHidden(current.machine.id, current.session)) }), () => {
          setFocus(tileKey(current.machine.id, current.pane.id));
          actions.setMode("wall");
        }, { hint: t("this agent takes the wall, the others wait in a strip"), icon: "maximize", keywords: ["wall", "focus", "big", "zoom"] });
      command("wall-office", office ? t("Wall: leave office mode") : t("Office mode on the Wall"), () => {
        setOffice(!office);
        if (office) leaveFull();
        else {
          actions.setMode("wall");
          enterFull();
        }
      }, { hint: office ? t("the grid back, the full screen left") : t("one agent large for a second screen — whoever needs you, else each in turn — the rest as cards; full screen"), icon: "monitor", keywords: ["wall", "office", "tv", "screen", "kiosk", "show", "featured", "big"] });
      command("wall-view", screens ? t("Wall of chats") : t("Wall of screens"), onTheWall({ view: screens ? "chat" : "screen" }), { hint: screens ? t("each tile the last turns of its conversation") : t("each tile its terminal as it is, scaled to fit"), icon: "layers", keywords: ["wall", "terminal", "screen", "chat", "view", "tiles"] });
      const zoo = wall.view === "zoo";
      command("wall-zoo", zoo ? t("Wall: leave the zoo") : t("Zoo on the Wall"), onTheWall({ view: zoo ? "chat" : "zoo" }), { hint: zoo ? t("each tile the last turns of its conversation") : t("each agent as a little creature whose pose says what it does, for a screen across the room"), icon: "layers", keywords: ["wall", "zoo", "creatures", "animals", "pets", "fun", "tv", "office", "view"] });
      if (current && !viewer && mode === "herdr") command("full", full ? t("Leave full screen") : t("Full screen"), full ? leaveFull : enterFull, { hint: full ? t("the header and the sidebar back") : t("the lens takes the whole screen; Esc leaves"), icon: full ? "minimize" : "maximize", keywords: ["fullscreen", "zen", "focus", "maximize", "screen"] });
      command("rail", footprint ? t("Hide the files rail") : t("Show the files rail"), actions.toggleFootprint, { hint: t("the files the agent touched, beside the lenses"), icon: "folder", keywords: ["footprint", "files"] });
      command("panels", t("Reset the panels' widths"), resetPanels, { hint: t("the sidebar, the files rail, the review queue and a task's detail back to their usual widths"), icon: "layers", keywords: ["resize", "width", "sidebar", "panel", "rail", "columns"] });
      command("activity", t("Activity"), () => actions.openPanel("activity"), { hint: t("what happened across the agents while you were away"), icon: "bell", keywords: ["feed", "history", "log", "news", "notifications", "away"] });
      command("broadcast", t("Message several agents…"), () => actions.openDialog({ kind: "broadcast" }), { hint: t("one text to the agents you tick, each in turn"), icon: "send", keywords: ["broadcast", "all", "every", "everyone", "prompt", "compact", "send"] });
      command("alerts", t("Alerts on this device"), () => actions.openPanel("alerts"), { hint: t("which pushes to get here"), icon: "bell", keywords: ["push", "notifications"] });
      command("start-recipe", t("New agent from recipe…"), () => actions.openDialog({ kind: "start-recipe", machineId: here, dir: current ? folderOf(current) : "" }), { hint: t("a saved agent with its model, arguments and first prompt, in a folder"), icon: "sparkles", keywords: ["recipe", "start", "agent", "saved", "preset", "template", "new"] });
      command("recipes", t("Recipes…"), () => actions.openDialog({ kind: "recipes" }), { hint: t("the saved ways to start an agent: agent, model, arguments, first prompt"), icon: "sparkles", keywords: ["recipe", "recipes", "preset", "template", "saved", "manage"] });
      command("schedules", t("Schedules"), () => actions.openPanel("schedules"), { hint: t("prompts sent to agents on a timetable"), icon: "calendar", keywords: ["cron", "timer", "prompt"] });
      const usageOn = (tab: UsageTab) => () => {
        writePref(USAGE_TAB_KEY, tab);
        actions.openPanel("usage");
      };
      command("usage", t("Usage report"), usageOn("spend"), { hint: t("what the agents cost, by day, project and session"), icon: "chart", keywords: ["cost", "tokens", "context", "money"] });
      command("scorecard", t("Models scorecard"), usageOn("models"), { hint: t("which model does your work best: finished runs, first-time approvals, merged pull requests, arenas kept, cost and time"), icon: "chart", keywords: ["model", "models", "compare", "best", "score", "quality", "approve", "merge", "opus", "sonnet", "fable", "gpt", "codex"] });
      command("standup", t("Standup: what every agent did today"), () => actions.openPanel("standup"), { hint: t("one report over every machine, ready to paste as Markdown"), icon: "sun", keywords: ["standup", "report", "today", "summary", "recap", "daily", "digest", "morning"] });
      command("arena-new", t("Arena: one prompt to several agents…"), () => actions.openDialog({ kind: "arena", at: current ? { machineId: current.machine.id, workspaceId: current.session.id } : undefined }), { hint: t("each in a worktree of its own, their work side by side, the best kept"), icon: "columns", keywords: ["arena", "compare", "best", "race", "parallel", "agents", "worktree", "versus"] });
      command("arenas", t("Arenas"), () => actions.openPanel("arena"), { hint: t("the lanes of every arena: how each agent stands, its changes, keep the best"), icon: "columns", keywords: ["arena", "compare", "lanes", "keep"] });
      command("open-lens-terminal", t("Notifications open: the terminal"), () => writePref(OPEN_LENS_KEY, "terminal"), { hint: `${openLens === "terminal" ? t("now · ") : ""}${t("a tapped notification, the Mac app's menu, an Activity row or Alt+N shows the pane's terminal")}`, icon: "terminal", keywords: ["notification", "alert", "open", "terminal", "lens"] });
      command("open-lens-chat", t("Notifications open: the chat"), () => writePref(OPEN_LENS_KEY, "chat"), { hint: `${openLens === "chat" ? t("now · ") : ""}${t("the same openings show the pane's chat")}`, icon: "message", keywords: ["notification", "alert", "open", "chat", "lens"] });
      command("open-lens-keep", t("Notifications open: the lens it was on"), () => writePref(OPEN_LENS_KEY, "keep"), { hint: `${openLens === "keep" ? t("now · ") : ""}${t("the same openings keep whichever lens the page was on")}`, icon: "columns", keywords: ["notification", "alert", "open", "keep", "lens"] });
      command("theme-system", t("Theme: follow the system"), () => setThemePref("system"), { hint: `${themePref === "system" ? t("now · ") : ""}${t("your dark flavour when the system is dark, Latte when it is light")}`, icon: "monitor", keywords: ["theme", "dark", "light", "catppuccin", "system", "auto"] });
      command("theme-dark", t("Theme: {flavor} (dark)", { flavor: FLAVOR_NAMES[darkFlavor] }), () => setThemePref("dark"), { hint: `${themePref === "dark" ? t("now · ") : ""}${t("Catppuccin {flavor}, whatever the system says", { flavor: FLAVOR_NAMES[darkFlavor] })}`, icon: "moon", keywords: ["theme", "dark", "catppuccin", "frappe", "macchiato", "mocha"] });
      command("theme-light", t("Theme: Latte (light)"), () => setThemePref("light"), { hint: `${themePref === "light" ? t("now · ") : ""}${t("Catppuccin Latte, whatever the system says")}`, icon: "sun", keywords: ["theme", "light", "catppuccin", "latte"] });
      command("settings", t("Settings"), () => actions.openDialog({ kind: "settings" }), { hint: t("the theme, the sidebar, the panels, the alerts, the keys"), icon: "sliders", keywords: ["settings", "preferences", "theme", "options"] });
      command("keys", t("Keyboard shortcuts"), () => actions.openDialog({ kind: "keys" }), { hint: t("? · every key the page answers"), icon: "keyboard", keywords: ["keys", "shortcuts", "help", "hotkeys", "bindings", "keyboard"] });
      command("float-terminal", float ? t("Hide the floating terminal") : t("Floating terminal"), () => toggleFloat(machines, current?.machine.id ?? null), {
        hint: float ? t("the shell keeps running · {key}", { key: keyWords(isMac) }) : t("a shell of its own over any screen, on this pane's machine · {key}", { key: keyWords(isMac) }),
        icon: "square-terminal",
        keywords: ["terminal", "shell", "float", "scratch", "console", "quake", "drawer"],
      });
      command("machine", t("Add a PC…"), () => actions.openDialog({ kind: "machine" }), { hint: t("a remote machine over SSH"), icon: "monitor", keywords: ["machine", "remote", "ssh", "host"] });
      command("budgets", t("Budgets…"), usageOn("spend"), { hint: t("a spending limit per day, week or month, with a push when it is near"), icon: "chart", keywords: ["budget", "limit", "cost", "spend", "money"] });
      if (current && !viewer && current.pane.agent === "claude") command("model", t("Switch this agent's model…"), () => actions.openDialog({ kind: "model", machineId: current.machine.id, paneId: current.pane.id, current: current.pane.usage?.name }), { hint: t("Opus, Sonnet, Haiku or an id, through Claude Code's own /model"), icon: "sparkles", keywords: ["model", "opus", "sonnet", "haiku", "switch", "cheaper", "faster"] });
      for (const flavor of DARK_FLAVORS) {
        command(`flavor-${flavor}`, t("Dark flavour: {flavor}", { flavor: FLAVOR_NAMES[flavor] }), () => setDarkFlavorPref(flavor), { hint: `${darkFlavor === flavor ? t("now · ") : ""}${t("Catppuccin {flavor} whenever the theme is dark", { flavor: FLAVOR_NAMES[flavor] })}`, icon: "moon", keywords: ["theme", "dark", "flavour", "flavor", "catppuccin", flavor] });
      }
      for (const size of TEXT_SIZES) {
        command(`text-${size}`, t("Text size: {size}", { size: TEXT_SIZE_NAMES[size] }), () => setTextSizePref(size), { hint: `${textSize === size ? t("now · ") : ""}${size === "normal" ? t("the page's type scale as designed; the terminal keeps its own size") : t("the page's type scale, {size}; the terminal keeps its own size", { size: TEXT_SIZE_NAMES[size].toLowerCase() })}`, icon: size === "small" ? "text-smaller" : "text-larger", keywords: ["text", "size", "font", "zoom", "bigger", "smaller", size] });
      }
      for (const language of LANGS) {
        command(`lang-${language}`, language === "ru" ? "Язык: Русский" : t("Language: English"), () => setLangPref(language), { hint: `${currentLang() === language ? t("now · ") : ""}${t("the page's own words; a change reloads the page")}`, icon: "monitor", keywords: ["language", "язык", "russian", "english", "русский", language] });
      }
      if (current?.pane.agent && !viewer && mode === "herdr") command("split", split ? t("Chat and terminal: one at a time") : t("Chat beside the terminal"), actions.toggleSplit, { hint: split ? t("the lens switch shows one lens again") : t("the chat on the left, the terminal on the right, on a screen at least 1180 px wide with a mouse"), icon: "columns", keywords: ["split", "side", "beside", "chat", "terminal", "columns", "both"] });
      command("sharing", share.on ? t("Screen-sharing mode: off") : t("Screen-sharing mode: on"), toggleSharing, {
        hint: `${SHARING_KEY_NAME} · ${share.on ? t("names and answers show again") : t("the folders and machines you marked, or everything, read as neutral names; their panes covered")}`,
        icon: share.on ? "eye" : "eye-off",
        keywords: ["sharing", "screen", "share", "privacy", "hide", "stream", "demo", "screenshot"],
      });
      if (share.on) command("sharing-all", share.pref.all ? t("While sharing: hide only what I marked") : t("While sharing: hide everything"), () => setSharingAll(!share.pref.all), { icon: "filter", keywords: ["sharing", "screen", "hide", "all", "marked"] });
      if (current && !viewer) {
        const folder = sessionFolder(current.session);
        const where = share.folderName(current.machine.id, folder, basename(folder) || current.session.name);
        if (folder) command("sharing-folder", markedFolder(share.pref, current.machine.id, folder) ? t("Show “{folder}” when sharing", { folder: where }) : t("Hide “{folder}” when sharing", { folder: where }), () => toggleSharingFolder(current.machine.id, folder), { hint: t("its name, branch, answers and panes, while the screen-sharing mode is on"), icon: "eye-off", keywords: ["sharing", "screen", "hide", "folder", "privacy"] });
        command("sharing-machine", markedMachine(share.pref, current.machine.id) ? t("Show {machine} when sharing", { machine: share.machineLabel(current.machine) }) : t("Hide {machine} when sharing", { machine: share.machineLabel(current.machine) }), () => toggleSharingMachine(current.machine.id), { hint: t("the machine and every folder on it, while the screen-sharing mode is on"), icon: "eye-off", keywords: ["sharing", "screen", "hide", "machine", "privacy"] });
      }
      const typed = query.trim();
      command("import-issue", t("New task from a GitHub issue…"), () => actions.openDialog({ kind: "import-issue" }), { hint: t("read with gh on the machine, as you; an agent can start on it at once"), icon: "link", keywords: ["github", "issue", "import", "task", "new", "gh"] });
      if (jiraSite) command("import-jira", t("New task from a Jira issue…"), () => actions.openDialog({ kind: "import-jira" }), { hint: t("read from your Jira site; an agent can start on it at once"), icon: "link", keywords: ["jira", "issue", "ticket", "import", "task", "new"] });
      command("search", typed ? t("Search every conversation for “{typed}”", { typed }) : t("Search every conversation…"), () => actions.openSearch(typed), { hint: t("what you or an agent said, on every machine"), icon: "search", keywords: ["find", "transcript"], always: true });
      const shown = share.on ? commands.map((entry) => ({ ...entry, title: share.redact(entry.title), hint: entry.hint && share.redact(entry.hint) })) : commands;
      return [...panes, ...(viewOnly ? shown.filter((entry) => viewerCommand(entry.id)) : shown)];
    },
    [machines, viewOnly, jiraSite, server, mode, lens, viewer, chatSession, chatLive, footprint, split, full, float, themePref, openLens, darkFlavor, textSize, current, radar, wall, setWall, focus, setFocus, office, setOffice, timelineShown, setTimelineShown, step, now, goNext, share],
  );
}
