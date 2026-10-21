// The app's own state: what is selected, which lens and mode are on, which overlays are open. Anything
// the server knows is not here (that is the roster); anything per device that should outlive a reload is
// a pref. Components read a field with app.use and change it through the actions beside it.

import { createStore } from "@/lib/store";
import { isTouch, readPref, writePref } from "@/lib/prefs";
import type { Machine, Session } from "@/features/roster/api";
import type { FileTarget } from "@/features/files/target";
import type { SearchHit } from "@/features/search/api";
import { readURL } from "./url";

export type Lens = "chat" | "terminal" | "changes";
export type Mode = "herdr" | "wall" | "tasks" | "code";
export type Selection = { machineId: string; paneId: string };

// Every "make something" dialog, keyed so only one is open at a time.
export type AppDialog =
  | { kind: "rename"; machineId: string; what: "tab" | "workspace"; id: string; label: string }
  | { kind: "workspace"; machineId: string }
  | { kind: "tab"; machineId: string; session: Session }
  | { kind: "worktree"; machineId: string; session: Session }
  | { kind: "machine" }
  | { kind: "renameMachine"; machine: Machine }
  | null;

export type RowMenu = { machine: Machine; session: Session; x: number; y: number } | null;
export type NoticeMsg = { text: string; error?: boolean; brief?: boolean };
export type Confirm = { text: string; label: string; run: () => void };
// a jump names its pane ("" = the transcript viewer): a Chat gets it only when it is that pane's
export type Jump = { machineId: string; paneId: string; session: string; turn: string; n: number };
export type Panel = "search" | "alerts" | "schedules" | "usage" | "release" | null;

export type AppState = {
  requested: Selection | null; // the selection is a request; the pane shown is derived from it, never fixed up in an effect
  lens: Lens;
  mode: Mode;
  drawer: boolean;
  dialog: AppDialog;
  menu: RowMenu;
  panel: Panel; // the one overlay panel open, if any
  notice: NoticeMsg | null;
  confirm: Confirm | null; // a dialog of our own: window.confirm() is unreliable inside embedded browsers
  file: { machineId: string; paneId: string; dir: string; target: FileTarget } | null; // dir stands in for the pane's folder when there is no pane
  viewer: SearchHit | null; // a transcript on its own, from a search hit
  jump: Jump | null;
  codeOpened: boolean; // the Code mode stays mounted once opened, so the editor never reloads
  popout: Selection | null; // the pane in the small window above every other app (app/pip.ts owns the window)
};

const fromURL = readURL();
const mode = fromURL.mode ?? (fromURL.requested ? "herdr" : readPref<Mode>("mode", "herdr"));

export const app = createStore<AppState>({
  requested: fromURL.requested,
  lens: fromURL.lens ?? (isTouch ? "chat" : "terminal"),
  mode,
  drawer: false,
  dialog: null,
  menu: null,
  panel: null,
  notice: null,
  confirm: null,
  file: null,
  viewer: null,
  jump: null,
  codeOpened: mode === "code",
  popout: null,
});

let jumps = 0;

export const actions = {
  // a pane asked for by a click in the sidebar, a notification, a search hit, a task
  selectPane(machineId: string, paneId: string) {
    app.set({ requested: { machineId, paneId }, viewer: null, drawer: false, mode: "herdr" });
    writePref("mode", "herdr");
  },
  setLens(lens: Lens) {
    app.set({ lens });
  },
  setMode(mode: Mode) {
    app.set((s) => ({ mode, codeOpened: s.codeOpened || mode === "code" }));
    writePref("mode", mode);
  },
  toggleDrawer() {
    app.set((s) => ({ drawer: !s.drawer }));
  },
  closeDrawer() {
    app.set({ drawer: false });
  },
  openDialog(dialog: AppDialog) {
    app.set({ dialog, menu: null });
  },
  closeDialog() {
    app.set({ dialog: null });
  },
  openMenu(menu: RowMenu) {
    app.set((s) => ({ menu: menu && s.menu?.session.id === menu.session.id ? null : menu }));
  },
  closeMenu() {
    app.set({ menu: null });
  },
  openPanel(panel: Panel) {
    app.set({ panel });
  },
  togglePanel(panel: Panel) {
    app.set((s) => ({ panel: s.panel === panel ? null : panel }));
  },
  closePanel() {
    app.set({ panel: null });
  },
  notify(text: string, opts: { error?: boolean; brief?: boolean } = {}) {
    app.set({ notice: { text, ...opts } });
  },
  clearNotice() {
    app.set({ notice: null });
  },
  // ask shows a yes/no before something destructive; run goes on a yes
  ask(text: string, label: string, run: () => void) {
    app.set({ confirm: { text, label, run } });
  },
  closeConfirm() {
    app.set({ confirm: null });
  },
  openFile(machineId: string, paneId: string, target: FileTarget, dir = "") {
    app.set({ file: { machineId, paneId, dir, target } });
  },
  retargetFile(target: FileTarget) {
    app.set((s) => ({ file: s.file ? { ...s.file, target } : s.file }));
  },
  closeFile() {
    app.set({ file: null });
  },
  // a transcript with no agent behind it, read only
  openViewer(hit: SearchHit) {
    app.set({ viewer: hit, mode: "herdr", panel: null });
    writePref("mode", "herdr");
  },
  closeViewer() {
    app.set({ viewer: null });
  },
  jumpTo(machineId: string, paneId: string, session: string, turn: string) {
    app.set({ jump: { machineId, paneId, session, turn, n: ++jumps }, lens: "chat" });
  },
  setPopOut(popout: Selection | null) {
    app.set({ popout });
  },
};
