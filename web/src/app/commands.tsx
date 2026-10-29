// What the palette offers: every pane to jump to, and every command the page has a button or a menu item
// for — the modes, the lenses, the dialogs, the panels, the rail, the pop-out, the search. The one place
// that lists them; the palette itself only shows and ranks.

import { useCallback } from "react";
import Palette from "@/features/palette/Palette";
import { paneEntries, type Entry } from "@/features/palette/rank";
import { exportConversation } from "@/features/chat/export";
import { useTimelineShown } from "@/features/chat/TimelineBar";
import { DEFAULT_RADAR, rowLabel, type RadarPrefs } from "@/features/roster/radar";
import { closeWorkspace } from "@/features/roster/WorkspaceMenu";
import { moving, useClock, useSpinner } from "@/features/roster/useClocks";
import { useRoster } from "@/features/roster/useRoster";
import { DEFAULT_WALL, tileKey, type WallPrefs } from "@/features/roster/wall";
import { agentTitle, askRestart } from "@/features/updates/restartPane";
import { isMac, usePref } from "@/lib/prefs";
import { basename, errMsg } from "@/lib/text";
import { resetPanels } from "@/ui";
import type { IconName } from "@/ui/icons";
import { folderOf, useCurrent } from "./current";
import { enterFull, leaveFull } from "./full";
import { NEXT_KEY, useNextNeedingMe } from "./next";
import { popOut, popOutSupported } from "./pip";
import { actions, app, type Lens, type Mode } from "./store";
import { setDarkFlavorPref, setTextSizePref, setThemePref, useDarkFlavorPref, useTextSize, useThemePref } from "./theme";
import { TEXT_SIZE_NAMES, TEXT_SIZES } from "@/lib/look";
import { currentLang, LANGS, t } from "@/lib/i18n";
import { setLangPref } from "./lang";
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
  const { machines } = useRoster();
  const mode = app.use((s) => s.mode);
  const lens = app.use((s) => s.lens);
  const viewer = app.use((s) => s.viewer);
  const chatSession = app.use((s) => s.chatSession);
  const chatLive = app.use((s) => s.chatLive);
  const footprint = app.use((s) => s.footprint);
  const full = app.use((s) => s.full);
  const themePref = useThemePref();
  const darkFlavor = useDarkFlavorPref();
  const textSize = useTextSize();
  const current = useCurrent();
  const [radar] = usePref<RadarPrefs>("radar", DEFAULT_RADAR);
  const [wall, setWall] = usePref<WallPrefs>("wall", DEFAULT_WALL);
  const [focus, setFocus] = usePref<string>("wall.focus", "");
  const [office, setOffice] = usePref<boolean>("wall.office", false);
  const [timelineShown, setTimelineShown] = useTimelineShown();
  const step = useSpinner(moving(machines));
  const now = useClock(machines);
  const goNext = useNextNeedingMe();
  return useCallback(
    (query: string) => {
      const selection = current ? { machineId: current.machine.id, paneId: current.pane.id } : null;
      const panes = paneEntries(machines, radar, now, step, selection, actions.selectPane);
      const commands: Entry[] = [];
      const command = (id: string, title: string, run: () => void, more: Partial<Entry> = {}) => commands.push({ id, kind: "command", title, keywords: [], run, ...more });

      command("next-needs", t("Go to the next agent that needs you"), goNext, { hint: `${NEXT_KEY} · ${t("a question open, a watchdog note, a finish not looked at")}`, icon: "arrow-right", keywords: ["next", "needs", "attention", "waiting", "blocked", "question", "done", "jump"] });
      for (const m of MODES) if (m.mode !== mode) command(`mode:${m.mode}`, m.title, () => actions.setMode(m.mode), { hint: m.hint, icon: m.icon, keywords: ["mode", m.mode] });
      if (current && !viewer) {
        const folder = folderOf(current);
        for (const l of LENSES) {
          if ((mode === "herdr" && l.lens === lens) || (l.lens === "changes" && !folder)) continue;
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
      if (current && !viewer) {
        const { machine, session, tab } = current;
        const named = `“${session.name}”`;
        command("tab", t("New tab in {workspace}…", { workspace: named }), () => actions.openDialog({ kind: "tab", machineId: machine.id, session }), { hint: t("another agent in the same folder"), icon: "plus", keywords: ["agent", "create"] });
        command("worktree", t("New worktree of {workspace}…", { workspace: named }), () => actions.openDialog({ kind: "worktree", machineId: machine.id, session }), { hint: t("a branch checked out beside it, an agent in it"), icon: "plus", keywords: ["branch", "create"] });
        command("rename-workspace", t("Rename workspace {workspace}…", { workspace: named }), () => actions.openDialog({ kind: "rename", machineId: machine.id, what: "workspace", id: session.id, label: session.name }), { icon: "tag" });
        command("rename-tab", `Rename tab ${tab.named ? `“${tab.label}”` : tab.number}…`, () => actions.openDialog({ kind: "rename", machineId: machine.id, what: "tab", id: tab.id, label: tab.named ? tab.label : "" }), { icon: "tag" });
        if (current.pane.agent) command("restart-pane", t("Restart {agent} on its session…", { agent: agentTitle(current.pane.agent) }), () => askRestart(machine.id, current.pane.id, current.pane.agent), { hint: t("its own exit, then started again on the same conversation"), icon: "refresh-cw", keywords: ["restart", "resume", "stuck", "hung", "reload", "again"] });
        command("close-workspace", t("Close workspace {workspace}", { workspace: named }), () => closeWorkspace(machine, session), { hint: t("its panes close and the agents in them stop"), icon: "x", danger: true, keywords: ["kill", "stop", "quit"] });
        if (popOutSupported) command("popout", t("Pop out this agent"), () => void popOut(selection!), { hint: t("a small window above every other app"), icon: "pip", keywords: ["picture", "window", "float"] });
        if (current.pane.agent) command("export", t("Export this conversation as Markdown"), () => void exportConversation(machine.id, current.pane.id, chatSession, folderOf(current), { agent: current.pane.agent, workspace: session.name, machine: machine.label }).catch(toastError), { hint: t("the session shown, as a file"), icon: "download", keywords: ["save", "download", "markdown", "transcript", "share", "file"] });
        if (current.pane.agent) command("recap", t("Recap this conversation"), () => actions.openDialog({ kind: "recap", machineId: machine.id, paneId: current.pane.id, session: chatSession, dir: folderOf(current), title: machine.id !== "local" ? `${session.name} on ${machine.label}` : session.name }), { hint: t("prompts, answers, tools, files, cost — Copy as Markdown"), icon: "sun", keywords: ["recap", "summary", "report", "standup", "digest", "what happened"] });
        if (current.pane.agent) command("handover", t("Continue this conversation with another agent"), () => actions.openDialog({ kind: "handover", machineId: machine.id, paneId: current.pane.id, session: chatSession, agent: current.pane.agent, workspace: machine.id !== "local" ? `${session.name} on ${machine.label}` : session.name }), { hint: t("a new tab here with Codex, Gemini or a fresh Claude, the gist of this conversation as its first prompt"), icon: "arrow-right", keywords: ["handover", "hand over", "continue", "switch", "codex", "gemini", "kimi", "another agent", "model", "cheaper", "stuck"] });
        if (current.pane.agent) command("pins", t("Pins in this conversation…"), () => actions.openDialog({ kind: "pins", machineId: machine.id, paneId: current.pane.id, dir: folderOf(current), shown: chatSession ?? chatLive, title: machine.id !== "local" ? `${session.name} on ${machine.label}` : session.name }), { hint: t("the turns you pinned in this folder's conversations, a click opens each"), icon: "pin", keywords: ["pin", "pins", "pinned", "bookmark", "bookmarks", "saved", "mark"] });
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
        command("wall-focus", t("Focus {agent} on the Wall", { agent: rowLabel(current.pane, current.tab, current.session.name, radar) }), () => {
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
      if (current && !viewer && mode === "herdr") command("full", full ? t("Leave full screen") : t("Full screen"), full ? leaveFull : enterFull, { hint: full ? t("the header and the sidebar back") : t("the lens takes the whole screen; Esc leaves"), icon: full ? "minimize" : "maximize", keywords: ["fullscreen", "zen", "focus", "maximize", "screen"] });
      command("rail", footprint ? t("Hide the files rail") : t("Show the files rail"), actions.toggleFootprint, { hint: t("the files the agent touched, beside the lenses"), icon: "folder", keywords: ["footprint", "files"] });
      command("panels", t("Reset the panels' widths"), resetPanels, { hint: t("the sidebar, the files rail, the review queue and a task's detail back to their usual widths"), icon: "layers", keywords: ["resize", "width", "sidebar", "panel", "rail", "columns"] });
      command("activity", t("Activity"), () => actions.openPanel("activity"), { hint: t("what happened across the agents while you were away"), icon: "bell", keywords: ["feed", "history", "log", "news", "notifications", "away"] });
      command("broadcast", t("Message several agents…"), () => actions.openDialog({ kind: "broadcast" }), { hint: t("one text to the agents you tick, each in turn"), icon: "send", keywords: ["broadcast", "all", "every", "everyone", "prompt", "compact", "send"] });
      command("alerts", t("Alerts on this device"), () => actions.openPanel("alerts"), { hint: t("which pushes to get here"), icon: "bell", keywords: ["push", "notifications"] });
      command("start-recipe", t("New agent from recipe…"), () => actions.openDialog({ kind: "start-recipe", machineId: here, dir: current ? folderOf(current) : "" }), { hint: t("a saved agent with its model, arguments and first prompt, in a folder"), icon: "sparkles", keywords: ["recipe", "start", "agent", "saved", "preset", "template", "new"] });
      command("recipes", t("Recipes…"), () => actions.openDialog({ kind: "recipes" }), { hint: t("the saved ways to start an agent: agent, model, arguments, first prompt"), icon: "sparkles", keywords: ["recipe", "recipes", "preset", "template", "saved", "manage"] });
      command("schedules", t("Schedules"), () => actions.openPanel("schedules"), { hint: t("prompts sent to agents on a timetable"), icon: "calendar", keywords: ["cron", "timer", "prompt"] });
      command("usage", t("Usage report"), () => actions.openPanel("usage"), { hint: t("what the agents cost, by day, project and session"), icon: "chart", keywords: ["cost", "tokens", "context", "money"] });
      command("standup", t("Standup: what every agent did today"), () => actions.openPanel("standup"), { hint: t("one report over every machine, ready to paste as Markdown"), icon: "sun", keywords: ["standup", "report", "today", "summary", "recap", "daily", "digest", "morning"] });
      command("theme-system", t("Theme: follow the system"), () => setThemePref("system"), { hint: `${themePref === "system" ? t("now · ") : ""}${t("your dark flavour when the system is dark, Latte when it is light")}`, icon: "monitor", keywords: ["theme", "dark", "light", "catppuccin", "system", "auto"] });
      command("theme-dark", t("Theme: {flavor} (dark)", { flavor: FLAVOR_NAMES[darkFlavor] }), () => setThemePref("dark"), { hint: `${themePref === "dark" ? t("now · ") : ""}${t("Catppuccin {flavor}, whatever the system says", { flavor: FLAVOR_NAMES[darkFlavor] })}`, icon: "moon", keywords: ["theme", "dark", "catppuccin", "frappe", "macchiato", "mocha"] });
      command("theme-light", t("Theme: Latte (light)"), () => setThemePref("light"), { hint: `${themePref === "light" ? t("now · ") : ""}${t("Catppuccin Latte, whatever the system says")}`, icon: "sun", keywords: ["theme", "light", "catppuccin", "latte"] });
      command("settings", t("Settings"), () => actions.openDialog({ kind: "settings" }), { hint: t("the theme, the sidebar, the panels, the alerts, the keys"), icon: "sliders", keywords: ["settings", "preferences", "theme", "options"] });
      command("keys", t("Keyboard shortcuts"), () => actions.openDialog({ kind: "keys" }), { hint: t("? · every key the page answers"), icon: "keyboard", keywords: ["keys", "shortcuts", "help", "hotkeys", "bindings", "keyboard"] });
      command("machine", t("Add a PC…"), () => actions.openDialog({ kind: "machine" }), { hint: t("a remote machine over SSH"), icon: "monitor", keywords: ["machine", "remote", "ssh", "host"] });
      command("budgets", t("Budgets…"), () => actions.openPanel("usage"), { hint: t("a spending limit per day, week or month, with a push when it is near"), icon: "chart", keywords: ["budget", "limit", "cost", "spend", "money"] });
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
      const typed = query.trim();
      command("import-issue", t("New task from a GitHub issue…"), () => actions.openDialog({ kind: "import-issue" }), { hint: t("read with gh on the machine, as you; an agent can start on it at once"), icon: "link", keywords: ["github", "issue", "import", "task", "new", "gh"] });
      command("search", typed ? t("Search every conversation for “{typed}”", { typed }) : t("Search every conversation…"), () => actions.openSearch(typed), { hint: t("what you or an agent said, on every machine"), icon: "search", keywords: ["find", "transcript"], always: true });
      return [...panes, ...commands];
    },
    [machines, mode, lens, viewer, chatSession, footprint, full, themePref, darkFlavor, textSize, current, radar, wall, setWall, focus, setFocus, office, setOffice, timelineShown, setTimelineShown, step, now, goNext],
  );
}
