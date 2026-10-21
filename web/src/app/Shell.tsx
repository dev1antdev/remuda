// The shell: the header, the sidebar and the pane area or the other modes (the wall, tasks, code), and the overlays. It also
// keeps the address in step with the selection, hears the service worker and the ⌘K key, and tells the
// server when a finished pane is being looked at.

import { useEffect } from "react";
import { markSeen } from "@/features/roster/api";
import Sidebar from "@/features/roster/Sidebar";
import { useRoster } from "@/features/roster/useRoster";
import { resyncPush } from "@/features/alerts/push";
import { readAlertPrefs } from "@/features/alerts/prefs";
import Tasks from "@/features/tasks/Tasks";
import Code from "@/features/code/Code";
import { Toasts } from "@/ui";
import Header from "./Header";
import Overlays from "./Overlays";
import PaneArea from "./PaneArea";
import PopOut from "./PopOut";
import Wall from "./Wall";
import { folderOf, useCurrent } from "./current";
import { actions, app } from "./store";
import { useToasts } from "./toasts";
import { writeURL } from "./url";

export default function Shell() {
  const mode = app.use((s) => s.mode);
  const lens = app.use((s) => s.lens);
  const drawer = app.use((s) => s.drawer);
  const viewer = app.use((s) => s.viewer);
  const codeOpened = app.use((s) => s.codeOpened);
  const current = useCurrent();
  const { machines } = useRoster();
  const toasts = useToasts();

  useEffect(() => {
    writeURL({ selection: current ? { machineId: current.machine.id, paneId: current.pane.id } : null, mode, lens });
  }, [current?.machine.id, current?.pane.id, mode, lens]); // eslint-disable-line react-hooks/exhaustive-deps

  useEffect(() => {
    // ⌘K on a Mac, Ctrl+K elsewhere, by key code whatever the layout; a terminal keeps its own Ctrl+K
    const mac = /Mac|iPhone|iPad/.test(navigator.platform);
    const onKey = (e: KeyboardEvent) => {
      if (e.code !== "KeyK" || e.altKey || e.shiftKey || (mac ? !e.metaKey : !e.ctrlKey)) return;
      if (!mac && (e.target as Element | null)?.closest?.(".xterm")) return;
      e.preventDefault();
      actions.togglePanel("search");
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, []);

  // a tapped notification in an already open tab arrives as a "select-pane" message from the service worker
  useEffect(() => {
    void resyncPush(readAlertPrefs());
    if (!("serviceWorker" in navigator)) return;
    const onMsg = (e: MessageEvent) => {
      const d = e.data as { type?: string; machine?: string; pane?: string };
      if (d?.type === "select-pane" && d.machine && d.pane) actions.selectPane(d.machine, d.pane);
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
    <div className="app">
      <Header current={current} />
      <div className="body">
        {codeOpened && (
          <div className="code-host" hidden={mode !== "code"}>
            <Code machines={machines} currentMachine={current?.machine.id ?? null} currentFolder={current ? folderOf(current) || null : null} visible={mode === "code"} />
          </div>
        )}
        {mode === "wall" && <Wall />}
        {mode === "tasks" && <Tasks />}
        {mode === "herdr" && (
          <>
            <aside className={drawer ? "sidebar open" : "sidebar"}>
              <Sidebar />
            </aside>
            {drawer && <div className="scrim" onClick={actions.closeDrawer} />}
            <PaneArea current={current} />
          </>
        )}
      </div>
      <Toasts items={toasts} />
      <Overlays />
      <PopOut />
    </div>
  );
}
