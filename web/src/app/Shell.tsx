// The shell: the rail, the header, the sidebar and the pane area or the other modes (the wall, tasks, code), and the overlays. It also
// keeps the address in step with the selection, hears the service worker, ⌘K (the palette), ⌘F (the find bar), ? (the keyboard
// sheet) and Alt+N (the next agent that needs you), reads the roster aloud when asked, keeps the browser tab's title and
// favicon counting the agents that need you, says what the agents did while the page was away, and tells the server when
// a finished pane is being looked at.

import { useEffect, useRef } from "react";
import { isMac } from "@/lib/prefs";
import { markSeen } from "@/features/roster/api";
import Sidebar from "@/features/roster/Sidebar";
import { useRoster } from "@/features/roster/useRoster";
import { resyncPush } from "@/features/alerts/push";
import { readAlertPrefs } from "@/features/alerts/prefs";
import { useVoice } from "@/features/alerts/useVoice";
import { useSyncPrefs } from "@/features/settings/useSyncPrefs";
import { useWelcomeBack } from "@/features/standup/useWelcomeBack";
import ReviewQueue from "@/features/review/ReviewQueue";
import Tasks from "@/features/tasks/Tasks";
import Code from "@/features/code/Code";
import { PANELS, Resizer, Toasts } from "@/ui";
import Header from "./Header";
import Overlays from "./Overlays";
import PaneArea from "./PaneArea";
import PopOut from "./PopOut";
import Rail from "./Rail";
import Wall from "./Wall";
import { folderOf, useCurrent } from "./current";
import { useFullKeys } from "./full";
import { useNextKey, useNextNeedingMe } from "./next";
import { handShell, inShell } from "@/lib/shell";
import { modeOf } from "./shellFeed";
import { actions, app } from "./store";
import { useBadge } from "./useBadge";
import { useShellFeed } from "./useShellFeed";
import { useToasts } from "./toasts";
import { panelOf, writeURL } from "./url";
import { t } from "@/lib/i18n";

export default function Shell() {
  const mode = app.use((s) => s.mode);
  const lens = app.use((s) => s.lens);
  const drawer = app.use((s) => s.drawer);
  const viewer = app.use((s) => s.viewer);
  const codeOpened = app.use((s) => s.codeOpened);
  const full = app.use((s) => s.full);
  const current = useCurrent();
  const { machines } = useRoster();
  const toasts = useToasts();
  useFullKeys();
  useNextKey(useNextNeedingMe());
  useVoice();
  useSyncPrefs();
  useWelcomeBack();
  useBadge();
  useShellFeed();

  useEffect(() => {
    if (!current && machines.length === 0) return; // the roster has not answered yet: the address keeps the pane it asked for
    writeURL({ selection: current ? { machineId: current.machine.id, paneId: current.pane.id } : null, mode, lens });
  }, [current?.machine.id, current?.pane.id, machines.length === 0, mode, lens]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    // ⌘K on a Mac, Ctrl+K elsewhere, by key code whatever the layout, opens the palette; a terminal keeps its own Ctrl+K.
    // ? (Shift and the slash key, by position) opens the keyboard sheet, never over a field or another dialog.
    const onKey = (e: KeyboardEvent) => {
      const target = e.target as Element | null;
      if (e.code === "Slash" && e.shiftKey && !e.metaKey && !e.ctrlKey && !e.altKey) {
        if (target?.closest?.("input, textarea, select, [contenteditable], .xterm")) return;
        const { dialog, panel } = app.get();
        if (dialog?.kind === "keys") {
          e.preventDefault();
          actions.closeDialog();
        } else if (!dialog && !panel) {
          e.preventDefault();
          actions.openDialog({ kind: "keys" });
        }
        return;
      }
      if (e.code !== "KeyK" || e.altKey || e.shiftKey || (isMac ? !e.metaKey : !e.ctrlKey)) return;
      if (!isMac && target?.closest?.(".xterm")) return;
      e.preventDefault();
      actions.togglePanel("palette");
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, []);

  // ⌘F / Ctrl+F, by key code, opens the find bar over the chat lens or the transcript viewer — from the composer too,
  // since no field there has a find of its own — unless an overlay is open; the terminal lens has it too, ⌘F on a Mac
  // and Ctrl+Shift+F elsewhere, where Ctrl+F is the shell's own
  const shown = useRef(current);
  shown.current = current;
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.code !== "KeyF" || e.altKey || (isMac ? !e.metaKey : !e.ctrlKey)) return;
      const s = app.get();
      if (s.mode !== "herdr" || s.dialog || s.panel || s.file || s.confirm) return;
      const terminal = !s.viewer && s.lens === "terminal" && !!shown.current;
      if (isMac ? e.shiftKey : e.shiftKey !== terminal) return;
      if (!s.viewer && !terminal && (s.lens !== "chat" || !shown.current)) return;
      e.preventDefault();
      actions.openFind();
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, []);

  // a tapped notification in an already open tab arrives as a "select-pane" message from the service worker, a digest's as "open-panel";
  // inside the Mac app the same arrives as a call on window.remudaShell, which the page's functions answer
  useEffect(() => {
    void resyncPush(readAlertPrefs());
    if (inShell()) {
      handShell({
        select: (machine, pane) => {
          if (!machine || !pane) return;
          actions.selectPane(machine, pane);
          actions.setLens("chat");
        },
        openPanel: (name) => {
          const panel = panelOf(name);
          if (panel) actions.openPanel(panel);
        },
        openMode: (name) => {
          const mode = modeOf(name);
          if (mode) actions.setMode(mode);
        },
        alert: () => {},
      });
    }
    if (!("serviceWorker" in navigator)) return;
    const onMsg = (e: MessageEvent) => {
      const d = e.data as { type?: string; machine?: string; pane?: string; panel?: string };
      if (d?.type === "select-pane" && d.machine && d.pane) actions.selectPane(d.machine, d.pane);
      if (d?.type === "open-panel" && panelOf(d.panel)) actions.openPanel(panelOf(d.panel));
    };
    navigator.serviceWorker.addEventListener("message", onMsg);
    return () => navigator.serviceWorker.removeEventListener("message", onMsg);
  }, []);

  // a finish while the pane is already open is only seen here (the server marks a terminal or chat open once); not while the tab is hidden
  const seenState = current?.pane.radar?.state;
  useEffect(() => {
    if (mode !== "herdr" || viewer || !current || seenState !== "done") return;
    const look = () => {
      if (document.visibilityState === "visible") void markSeen(current.machine.id, current.pane.id).catch(() => {});
    };
    look();
    document.addEventListener("visibilitychange", look);
    return () => document.removeEventListener("visibilitychange", look);
  }, [mode, !!viewer, current?.machine.id, current?.pane.id, seenState]); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className={full && (mode === "herdr" || mode === "wall") ? "app is-full" : "app"}>
      <Rail />
      <div className="app-main">
        <Header current={current} />
        <div className="body">
        {codeOpened && (
          <div className="code-host" hidden={mode !== "code"}>
            <Code
              machines={machines}
              currentMachine={current?.machine.id ?? null}
              currentFolder={current ? folderOf(current) || null : null}
              currentPane={current ? { machineId: current.machine.id, paneId: current.pane.id, dir: folderOf(current), agent: current.pane.agent } : null}
              visible={mode === "code"}
            />
          </div>
        )}
        {mode === "wall" && <Wall />}
        {mode === "tasks" && <Tasks />}
        {mode === "review" && <ReviewQueue />}
        {mode === "herdr" && (
          <>
            <aside className={drawer ? "sidebar open" : "sidebar"}>
              <Sidebar />
            </aside>
            <Resizer spec={PANELS.sidebar} edge="right" label={t("Sidebar width")} />
            {drawer && <div className="scrim" onClick={actions.closeDrawer} />}
            <PaneArea current={current} />
          </>
        )}
        </div>
      </div>
      <Toasts items={toasts} />
      <Overlays />
      <PopOut />
    </div>
  );
}
