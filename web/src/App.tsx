import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import {
  addMachine,
  closeTab,
  createSession,
  createTab,
  createWorktree,
  fetchRoster,
  killSession,
  markSeen,
  removeMachine,
  renameSession,
  renameTab,
  submitPrompt,
  type AgentKind,
  type Created,
  type Machine,
  type Pane,
  type Session,
  type Status,
  type Tab,
} from "./api";
import AlertsPanel from "./AlertsPanel";
import Chat from "./Chat";
import FileViewer, { type FileTarget } from "./FileViewer";
import KeyBar from "./KeyBar";
import { errMsg, isTouch, readAlertPrefs, usePref } from "./prefs";
import { resyncPush } from "./push";
import TerminalView, { type Modifier, type SizeMode, type TermApi } from "./Terminal";
import Tasks, { type AgentTarget } from "./Tasks";
import Code from "./Code";
import Review from "./Review";
import Schedules from "./Schedules";
import Sidebar from "./Sidebar";
import Select from "./Select";
import Logo from "./Logo";
import { DEFAULT_RADAR, type RadarPrefs } from "./radar";
import Icon from "./icons";

type Lens = "chat" | "terminal" | "changes";
type Selection = { machineId: string; paneId: string };
type Located = { machine: Machine; session: Session; tab: Tab; pane: Pane };

// Every "make something" dialog, keyed so only one is open at a time.
type Dialog =
  | { kind: "rename"; machineId: string; what: "tab" | "workspace"; id: string; label: string }
  | { kind: "workspace"; machineId: string }
  | { kind: "tab"; machineId: string; session: Session }
  | { kind: "worktree"; machineId: string; session: Session }
  | { kind: "machine" }
  | null;

// minimum terminal widths on offer, in columns (0 = fit the screen)
const MIN_WIDTHS = [0, 120, 160, 200, 240, 320];

type RowMenu = { machine: Machine; session: Session; x: number; y: number } | null;

const ADJ = ["clear", "quiet", "amber", "brisk", "calm", "early", "fresh", "keen", "mild", "swift"];
const NOUN = ["forest", "river", "harbor", "meadow", "summit", "valley", "ridge", "grove", "lake", "field"];
function suggestBranch(): string {
  const pick = (a: string[]) => a[Math.floor(Math.random() * a.length)]!;
  return `worktree/${pick(ADJ)}-${pick(NOUN)}-${1000 + Math.floor(Math.random() * 9000)}`;
}

// What the URL asks to open: ?machine=&pane= (a tapped notification).
type Requested = Selection;

function requestedFromURL(): Requested | null {
  const q = new URLSearchParams(location.search);
  const machineId = q.get("machine");
  const paneId = q.get("pane");
  if (machineId || paneId) history.replaceState(null, "", "/");
  if (!paneId) return null;
  return { machineId: machineId || "local", paneId };
}

function matches(x: Located, r: Requested): boolean {
  return x.machine.id === r.machineId && x.pane.id === r.paneId;
}

export default function App() {
  const [machines, setMachines] = useState<Machine[]>([]);
  const [agents, setAgents] = useState<AgentKind[]>([]);
  const rosterEtag = useRef<string | null>(null);
  // The selection is a request; what is shown is derived: the requested
  // pane when the roster has it (a tapped notification may ask before the
  // roster loads), else the first agent pane.
  const [requested, setRequested] = useState<Requested | null>(requestedFromURL);
  const [lens, setLens] = useState<Lens>(isTouch ? "chat" : "terminal");
  const [drawer, setDrawer] = useState(false);
  const [offline, setOffline] = useState(false);
  const [dialog, setDialog] = useState<Dialog>(null);
  const [menu, setMenu] = useState<RowMenu>(null);
  const [alerts, setAlerts] = useState(false);
  const [notice, setNotice] = useState<string | null>(null);
  // an in-app confirmation: window.confirm() returns false without asking
  // inside an embedded browser (JCEF), so nothing destructive may rely on it
  const [confirmReq, setConfirmReq] = useState<{ text: string; label: string; run: () => void } | null>(null);
  const ask = (text: string, label: string, run: () => void) => setConfirmReq({ text, label, run });
  const [termApi, setTermApi] = useState<TermApi | null>(null);
  const [modifier, setModifier] = useState<Modifier>(null);
  // selection mode: a plain drag selects text instead of reaching the agent
  const [selectMode, setSelectMode] = useState(false);
  const copied = useCallback((n: number) => setNotice(`Copied ${n} character${n === 1 ? "" : "s"}`), []);
  const [file, setFile] = useState<{ machineId: string; paneId: string; target: FileTarget } | null>(null);
  const [keybar, setKeybar] = usePref("keybar", isTouch);
  // A phone keeps the pane's own grid by default: resizing the shared pane
  // to a phone would squeeze the desktop's view of it too.
  const [sizeMode, setSizeMode] = usePref<SizeMode>("sizeMode", isTouch ? "keep" : "fit");
  // what the window is for right now: herdr's sessions, or the task list
  const [mode, setMode] = usePref<"herdr" | "tasks" | "code">("mode", "herdr");
  // the editor stays mounted once opened, so coming back does not reload it
  const [codeOpened, setCodeOpened] = useState(false);
  const [schedules, setSchedules] = useState(false);
  useEffect(() => {
    if (mode === "code") setCodeOpened(true);
  }, [mode]);
  // a pane asked for from outside (a tapped notification, a deep link) is
  // an Agents-mode thing, whatever the device was left in
  const requestPane = (r: Requested) => {
    setRequested(r);
    setMode("herdr");
  };
  // …including the one the page was opened with
  useEffect(() => {
    if (requested) setMode("herdr");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  const [termFont, setTermFont] = usePref("termFont", isTouch ? 12 : 13);
  // fit mode: a grid at least this wide, scrolling sideways on a narrower
  // screen — for diff viewers and tables that only truncate
  const [minCols, setMinCols] = usePref("minCols", 0);
  const [widthMenu, setWidthMenu] = useState<{ x: number; y: number } | null>(null);
  // machines folded shut in the sidebar, like herdr's collapsed groups
  const [collapsed, setCollapsed] = usePref<Record<string, boolean>>("collapsed", {});
  // how the sidebar reads and orders the agents (see radar.ts)
  const [radarPrefs, setRadarPrefs] = usePref<RadarPrefs>("radar", DEFAULT_RADAR);

  const refresh = useCallback(async () => {
    try {
      const res = await fetchRoster(rosterEtag.current);
      if (res) {
        rosterEtag.current = res.etag;
        setMachines(res.roster.machines);
        setAgents(res.roster.agents);
      }
      setOffline(false);
    } catch {
      setOffline(true);
    }
  }, []);

  useEffect(() => {
    void refresh();
    const t = setInterval(() => {
      if (document.visibilityState === "visible") void refresh();
    }, 3000);
    const onVis = () => {
      if (document.visibilityState === "visible") void refresh();
    };
    document.addEventListener("visibilitychange", onVis);
    return () => {
      clearInterval(t);
      document.removeEventListener("visibilitychange", onVis);
    };
  }, [refresh]);

  // push: re-register this device on load; a tapped notification in an
  // already open tab arrives as a message from the service worker. A host
  // (the IDE plugin) can also ask for the pane of a folder.
  useEffect(() => {
    void resyncPush(readAlertPrefs());
    if (!("serviceWorker" in navigator)) return;
    const onMsg = (e: MessageEvent) => {
      const d = e.data as { type?: string; machine?: string; pane?: string };
      if (d?.type === "select-pane" && d.machine && d.pane) requestPane({ machineId: d.machine, paneId: d.pane });
    };
    navigator.serviceWorker.addEventListener("message", onMsg);
    return () => {
      navigator.serviceWorker.removeEventListener("message", onMsg);
    };
  }, []);

  // A menu closes on any click elsewhere or Escape.
  const menuOpen = !!menu || !!widthMenu;
  useEffect(() => {
    if (!menuOpen) return;
    const close = () => {
      setMenu(null);
      setWidthMenu(null);
    };
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && close();
    document.addEventListener("click", close);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("click", close);
      document.removeEventListener("keydown", onKey);
    };
  }, [menuOpen]);

  useEffect(() => {
    if (!notice) return;
    const t = setTimeout(() => setNotice(null), notice.startsWith("Copied") ? 2000 : 10000);
    return () => clearTimeout(t);
  }, [notice]);

  const allPanes = useMemo(() => {
    const out: Located[] = [];
    for (const machine of machines)
      for (const session of machine.sessions)
        for (const tab of session.tabs) for (const pane of tab.panes) out.push({ machine, session, tab, pane });
    return out;
  }, [machines]);

  const current = useMemo(
    () =>
      (requested && (allPanes.find((x) => matches(x, requested) && x.pane.agent) ?? allPanes.find((x) => matches(x, requested)))) ??
      allPanes.find((x) => x.pane.agent) ??
      allPanes[0] ??
      null,
    [allPanes, requested],
  );

  // what the Tasks mode can send a task to
  const agentTargets = useMemo<AgentTarget[]>(
    () =>
      allPanes
        .filter((x) => x.pane.agent)
        .map((x) => ({
          machineId: x.machine.id,
          machineLabel: x.machine.label,
          sessionName: x.session.name,
          paneTitle: x.tab.named ? x.tab.label : x.pane.title || x.tab.label,
          paneId: x.pane.id,
          agent: x.pane.agent,
          status: x.pane.status,
          cwd: x.pane.cwd,
        })),
    [allPanes],
  );

  // Looking at a pane counts as having seen it finish: the server marks a
  // terminal or chat open once, but a finish while the terminal stays open
  // is only seen here. Not while the tab is hidden — a phone in a pocket
  // has not looked.
  const seenState = current?.pane.radar?.state;
  useEffect(() => {
    if (mode !== "herdr" || !current || seenState !== "done") return;
    const look = () => {
      if (document.visibilityState === "visible") void markSeen(current.machine.id, current.pane.id).catch(() => {});
    };
    look();
    document.addEventListener("visibilitychange", look);
    return () => document.removeEventListener("visibilitychange", look);
  }, [mode, current?.machine.id, current?.pane.id, seenState]); // eslint-disable-line react-hooks/exhaustive-deps

  const selectPane = useCallback((machineId: string, paneId: string) => {
    setRequested({ machineId, paneId });
    setDrawer(false);
  }, []);

  // stable for the memoized chat turns and the terminal's link provider
  const currentRef = useRef(current);
  currentRef.current = current;
  const openFile = useCallback((target: FileTarget) => {
    const c = currentRef.current;
    if (!c) return;
    setFile({ machineId: c.machine.id, paneId: c.pane.id, target });
  }, []);
  const closeFile = useCallback(() => setFile(null), []);
  const retargetFile = useCallback((target: FileTarget) => setFile((f) => (f ? { ...f, target } : f)), []);
  const modifierUsed = useCallback(() => setModifier(null), []);

  // After creating something, close the dialog, land on the new pane and
  // surface a warning — the container exists even when its agent did not
  // start, so the dialog must not stay open to be submitted twice.
  const landOn = async (machineId: string, made: Created) => {
    setDialog(null);
    await refresh();
    selectPane(machineId, made.pane);
    if (made.warning) setNotice(made.warning);
  };

  const closeWorkspace = (m: Machine, s: Session) => {
    const n = s.tabs.reduce((a, t) => a + t.panes.length, 0);
    ask(`Close “${s.name}”? ${n} pane${n === 1 ? "" : "s"} close and the agents in them stop.`, "Close workspace", () =>
      void killSession(m.id, s.id).then(refresh),
    );
  };

  return (
    <div className="app">
      <header className="topbar">
        <button className="burger icon-btn" onClick={() => setDrawer((d) => !d)} title="Menu">
          <Icon name="menu" size={18} />
        </button>
        <nav className="mode-switch">
          <button className={mode === "herdr" ? "on" : ""} onClick={() => setMode("herdr")} title="Your agents and terminals, from herdr">
            Agents
          </button>
          <button className={mode === "tasks" ? "on" : ""} onClick={() => setMode("tasks")} title="Your Vikunja tasks">
            Tasks
          </button>
          <button className={mode === "code" ? "on" : ""} onClick={() => setMode("code")} title="VS Code on a machine, through herdr-ui">
            Code
          </button>
        </nav>
        <span className="title">
          {mode !== "herdr" ? null : current ? (
            <>
              {current.machine.id !== "local" && (
                <span className="title-machine">{current.machine.label} ›</span>
              )}
              {current.session.name}
              <span className="title-agent"> · {current.pane.agent || "shell"}</span>
            </>
          ) : (
            "herdr-ui"
          )}
        </span>
        {mode === "herdr" && current && (
          <nav className="lens-switch">
            <button className={lens === "chat" ? "on" : ""} onClick={() => setLens("chat")}>
              Chat
            </button>
            <button
              className={lens === "terminal" ? "on" : ""}
              onClick={() => setLens("terminal")}
            >
              Terminal
            </button>
            {(current.session.dir || current.pane.cwd) && (
              <button className={lens === "changes" ? "on" : ""} onClick={() => setLens("changes")} title="What changed in this workspace's folder since you last looked">
                Changes
                {current.session.review?.fresh ? <span className="lens-badge">{current.session.review.fresh}</span> : null}
              </button>
            )}
            {lens === "terminal" && (
              <>
                <span className="lens-sep" />
                <button
                  className={`kb-toggle${sizeMode === "keep" ? " on" : ""}`}
                  title={
                    sizeMode === "keep"
                      ? `Keeping the pane's own size (${current.pane.cols}×${current.pane.rows}); click to resize it to this screen`
                      : "Resizing the shared pane to this screen; click to keep its own size"
                  }
                  onClick={() => setSizeMode(sizeMode === "keep" ? "fit" : "keep")}
                >
                  <Icon name={sizeMode === "keep" ? "minimize" : "maximize"} />
                </button>
                {sizeMode === "fit" && (
                  <button
                    className={`kb-toggle${minCols ? " on" : ""}`}
                    title={
                      minCols
                        ? `At least ${minCols} columns wide; a narrower screen scrolls sideways`
                        : "Minimum width: make the terminal wider than the screen for diffs and tables, scrolling sideways"
                    }
                    onClick={(e) => {
                      e.stopPropagation();
                      const r = e.currentTarget.getBoundingClientRect();
                      setWidthMenu(widthMenu ? null : { x: r.left, y: r.bottom });
                    }}
                  >
                    <Icon name="move-horizontal" />
                    {minCols ? <span className="kb-toggle-num">{minCols}</span> : null}
                  </button>
                )}
                <button
                  className={`kb-toggle${selectMode ? " on" : ""}`}
                  title={
                    selectMode
                      ? "Selection mode is on: drag selects text and copies it. Click to give the mouse back to the agent."
                      : "Selection mode: drag to select and copy text (otherwise hold Option/Alt while dragging)"
                  }
                  onClick={() => setSelectMode((v) => !v)}
                >
                  <Icon name="text-select" />
                </button>
                <button className="kb-toggle kb-font" title="Smaller text" onClick={() => setTermFont((f) => Math.max(8, f - 1))}>
                  <Icon name="text-smaller" />
                </button>
                <button className="kb-toggle kb-font" title="Larger text" onClick={() => setTermFont((f) => Math.min(20, f + 1))}>
                  <Icon name="text-larger" />
                </button>
                <button
                  className={`kb-toggle${keybar ? " on" : ""}`}
                  title="Key bar"
                  onClick={() => setKeybar((k) => !k)}
                >
                  <Icon name="keyboard" />
                </button>
              </>
            )}
          </nav>
        )}
        {offline && <span className="pill pill-off">reconnecting…</span>}
        {notice && (
          <span className="pill pill-warn" title={notice} onClick={() => setNotice(null)}>
            {notice}
          </span>
        )}
        <button
          className={`bell${alerts ? " on" : ""}`}
          title="Alerts on this device"
          onClick={(e) => {
            e.stopPropagation();
            setAlerts((a) => !a);
          }}
        >
          <Icon name="bell" size={17} />
        </button>
      </header>
      {alerts && (
        <>
          <div className="scrim clear" onClick={() => setAlerts(false)} />
          <AlertsPanel onClose={() => setAlerts(false)} />
        </>
      )}

      <div className="body">
        {codeOpened && (
          <div className="code-host" hidden={mode !== "code"}>
            <Code
              machines={machines}
              currentMachine={current?.machine.id ?? null}
              currentFolder={current ? current.session.worktree?.checkout_path || current.pane.cwd || null : null}
              visible={mode === "code"}
            />
          </div>
        )}
        {mode === "tasks" && (
          <Tasks
            drawer={drawer}
            closeDrawer={() => setDrawer(false)}
            ask={ask}
            agents={agentTargets}
            currentPane={current ? { machineId: current.machine.id, paneId: current.pane.id } : null}
            onOpenPane={(machineId, paneId) => requestPane({ machineId, paneId })}
            onSend={(machineId, paneId, text) => submitPrompt(machineId, paneId, text)}
          />
        )}
        {mode === "herdr" && (
        <aside className={drawer ? "sidebar open" : "sidebar"}>
          <Sidebar
            machines={machines}
            current={current ? { machineId: current.machine.id, sessionId: current.session.id, paneId: current.pane.id } : null}
            prefs={radarPrefs}
            collapsed={collapsed}
            onToggle={(id) => setCollapsed((c) => ({ ...c, [id]: !c[id] }))}
            onSelect={selectPane}
            onMenu={(m, s, x, y) => setMenu(menu?.session.id === s.id ? null : { machine: m, session: s, x, y })}
            onNewWorkspace={(m) => setDialog({ kind: "workspace", machineId: m.id })}
            onRemoveMachine={(m) =>
              ask(`Remove machine “${m.label}”? Nothing on the PC is touched.`, "Remove", () => void removeMachine(m.id).then(refresh))
            }
            onAddMachine={() => setDialog({ kind: "machine" })}
            onPrefs={setRadarPrefs}
            onSchedules={() => setSchedules(true)}
          />
        </aside>
        )}
        {mode === "herdr" && drawer && <div className="scrim" onClick={() => setDrawer(false)} />}

        {mode === "herdr" && (
        <main className="pane-area">
          {current ? (
            <>
              <TabStrip
                session={current.session}
                tab={current.tab}
                pane={current.pane}
                onSelect={(paneId) => selectPane(current.machine.id, paneId)}
                onNewTab={() =>
                  setDialog({ kind: "tab", machineId: current.machine.id, session: current.session })
                }
                onRename={(t, label) => void renameTab(current.machine.id, t.id, label).then(refresh, (e) => setNotice(errMsg(e)))}
                onCloseTab={(t) => {
                  const last = current.session.tabs.length === 1;
                  const msg = last
                    ? `Close the last tab? Workspace “${current.session.name}” closes with it.`
                    : `Close tab “${t.label}”? Its ${t.panes.length} pane${t.panes.length === 1 ? "" : "s"} and agents stop.`;
                  ask(msg, "Close tab", () => void closeTab(current.machine.id, t.id).then(refresh));
                }}
              />
              {lens === "changes" ? (
                <Review
                  key={`${current.machine.id}|${current.session.id}`}
                  machineId={current.machine.id}
                  paneId={current.pane.id}
                  dir={current.session.dir || current.pane.cwd}
                  agent={current.pane.agent}
                  onOpenFile={(path, line) => openFile({ path, line })}
                />
              ) : lens === "chat" ? (
                <Chat
                  key={`${current.machine.id}|${current.pane.id}`} // fresh state per pane: no stale session pick
                  machineId={current.machine.id}
                  paneId={current.pane.id}
                  status={current.pane.status}
                  onOpenFile={openFile}
                  agents={agentTargets}
                  origin={`${current.session.name}${current.machine.id !== "local" ? ` on ${current.machine.label}` : ""}`}
                  dir={current.session.dir || current.pane.cwd}
                />
              ) : (
                <>
                  <TerminalView
                    machineId={current.machine.id}
                    paneId={current.pane.id}
                    cols={current.pane.cols}
                    rows={current.pane.rows}
                    agent={current.pane.agent !== ""}
                    mode={sizeMode}
                    minCols={minCols}
                    fontSize={termFont}
                    modifier={modifier}
                    selectMode={selectMode}
                    onModifierUsed={modifierUsed}
                    onReady={setTermApi}
                    onOpenFile={openFile}
                    onCopied={copied}
                  />
                  {keybar && <KeyBar term={termApi} modifier={modifier} setModifier={setModifier} />}
                </>
              )}
            </>
          ) : (
            <div className="placeholder">
              <p>No pane selected.</p>
              <button onClick={() => setDialog({ kind: "workspace", machineId: "local" })}>
                Start an agent
              </button>
            </div>
          )}
        </main>
        )}
      </div>

      {file && (
        <FileViewer
          machineId={file.machineId}
          paneId={file.paneId}
          target={file.target}
          onClose={closeFile}
          onRetarget={retargetFile}
        />
      )}

      {menu && (
        <div className="row-menu" style={{ left: menu.x, top: menu.y }} onClick={(e) => e.stopPropagation()}>
          <button onClick={() => { setMenu(null); setDialog({ kind: "tab", machineId: menu.machine.id, session: menu.session }); }}>
            New tab…
          </button>
          <button onClick={() => { setMenu(null); setDialog({ kind: "worktree", machineId: menu.machine.id, session: menu.session }); }}>
            New worktree…
          </button>
          <button onClick={() => { setMenu(null); setDialog({ kind: "rename", machineId: menu.machine.id, what: "workspace", id: menu.session.id, label: menu.session.name }); }}>
            Rename workspace…
          </button>
          {(() => {
            const t = menu.session.tabs.find((x) => x.id === menu.session.active_tab_id) ?? menu.session.tabs[0];
            return t ? (
              <button onClick={() => { setMenu(null); setDialog({ kind: "rename", machineId: menu.machine.id, what: "tab", id: t.id, label: t.named ? t.label : "" }); }}>
                Rename tab {t.named ? `“${t.label}”` : t.number}…
              </button>
            ) : null;
          })()}
          <button className="danger" onClick={() => { setMenu(null); closeWorkspace(menu.machine, menu.session); }}>
            Close workspace
          </button>
        </div>
      )}

      {dialog?.kind === "rename" && (
        <RenameDialog
          what={dialog.what}
          label={dialog.label}
          onClose={() => setDialog(null)}
          onRename={async (label) => {
            await (dialog.what === "tab" ? renameTab : renameSession)(dialog.machineId, dialog.id, label);
            setDialog(null);
            await refresh();
          }}
        />
      )}

      {widthMenu && (
        <div className="row-menu width-menu" style={{ left: widthMenu.x, top: widthMenu.y }} onClick={(e) => e.stopPropagation()}>
          {MIN_WIDTHS.map((n) => (
            <button
              key={n}
              className={n === minCols ? "on" : ""}
              onClick={() => {
                setMinCols(n);
                setWidthMenu(null);
              }}
            >
              {n ? `At least ${n} columns` : "Fit the screen"}
            </button>
          ))}
        </div>
      )}

      {schedules && <Schedules machines={machines} onClose={() => setSchedules(false)} ask={ask} />}

      {confirmReq && (
        <ConfirmDialog
          text={confirmReq.text}
          label={confirmReq.label}
          onClose={() => setConfirmReq(null)}
          onConfirm={() => {
            const run = confirmReq.run;
            setConfirmReq(null);
            run();
          }}
        />
      )}

      {dialog?.kind === "workspace" && (
        <NewWorkspaceDialog
          machines={machines}
          agents={agents}
          defaultMachine={dialog.machineId}
          onClose={() => setDialog(null)}
          onCreate={async (machine, dir, name, agent) => {
            await landOn(machine, await createSession({ machine, name, dir, agent }));
          }}
        />
      )}
      {dialog?.kind === "tab" && (
        <NewTabDialog
          session={dialog.session}
          agents={agents}
          onClose={() => setDialog(null)}
          onCreate={async (name, agent) => {
            await landOn(
              dialog.machineId,
              await createTab({ machine: dialog.machineId, workspace: dialog.session.id, name, agent }),
            );
          }}
        />
      )}
      {dialog?.kind === "worktree" && (
        <NewWorktreeDialog
          session={dialog.session}
          agents={agents}
          onClose={() => setDialog(null)}
          onCreate={async (branch, base, name, agent) => {
            await landOn(
              dialog.machineId,
              await createWorktree({
                machine: dialog.machineId, workspace: dialog.session.id, branch, base, name, agent,
              }),
            );
          }}
        />
      )}
      {dialog?.kind === "machine" && (
        <AddMachineDialog
          onClose={() => setDialog(null)}
          onAdd={async (label, host) => {
            await addMachine({ label, host });
            setDialog(null);
            await refresh();
          }}
        />
      )}
    </div>
  );
}

// Tabs of the current workspace sit above the pane, as in herdr; a split tab
// lists its panes under the tab row, connected like herdr's tree.
function TabStrip({
  session,
  tab,
  pane,
  onSelect,
  onNewTab,
  onCloseTab,
  onRename,
}: {
  session: Session;
  tab: Tab;
  pane: Pane;
  onSelect: (paneId: string) => void;
  onNewTab: () => void;
  onCloseTab: (t: Tab) => void;
  onRename: (t: Tab, label: string) => void;
}) {
  // double-click a tab to name it in place; Enter keeps, Escape drops
  const [editing, setEditing] = useState<{ id: string; value: string } | null>(null);
  const commit = () => {
    if (!editing) return;
    const t = session.tabs.find((x) => x.id === editing.id);
    const v = editing.value.trim();
    setEditing(null);
    if (t && v && v !== (t.named ? t.label : "")) onRename(t, v);
  };
  return (
    <div className="tabstrip">
      <div className="tabs" role="tablist">
        {session.tabs.map((t) => {
          const first = t.panes.find((p) => p.focused) ?? t.panes[0];
          if (editing?.id === t.id) {
            return (
              <div key={t.id} role="tab" className="tab sel editing">
                <input
                  className="tab-edit"
                  autoFocus
                  value={editing.value}
                  placeholder={`Tab ${t.number}`}
                  onChange={(e) => setEditing({ id: t.id, value: e.target.value })}
                  onBlur={commit}
                  onKeyDown={(e) => {
                    if (e.key === "Enter") commit();
                    if (e.key === "Escape") setEditing(null);
                  }}
                />
              </div>
            );
          }
          return (
            <div key={t.id} role="tab" aria-selected={t.id === tab.id} className={`tab${t.id === tab.id ? " sel" : ""}`}>
              <button
                className="tab-main"
                disabled={!first}
                title={t.named ? `${t.label} — double-click to rename` : "Double-click to name this tab"}
                onClick={() => first && onSelect(first.id)}
                onDoubleClick={() => setEditing({ id: t.id, value: t.named ? t.label : "" })}
              >
                <StatusDot status={t.status} />
                <span className="tab-num">{t.number}</span>
                {t.named && <span className="tab-label">{t.label}</span>}
                {t.panes.length > 1 && <span className="tab-count">{t.panes.length}</span>}
              </button>
              <button className="tab-close" title="Close tab" onClick={() => onCloseTab(t)}>
                <Icon name="x" size={13} />
              </button>
            </div>
          );
        })}
        <button className="tab-add" title="New tab in this workspace" onClick={onNewTab}>
          <Icon name="plus" size={14} />
        </button>
      </div>
      {tab.panes.length > 1 && (
        <div className="split-panes">
          {tab.panes.map((p, i) => (
            <button
              key={p.id}
              className={`split-pane${p.id === pane.id ? " sel" : ""}`}
              onClick={() => onSelect(p.id)}
              title={p.cwd}
            >
              <span className="tree-glyph">{i === tab.panes.length - 1 ? "└" : "├"}</span>
              <StatusDot status={p.status} />
              <span className="split-label">{p.title}</span>
              {p.agent && <span className={`ws-agent agent-${p.agent}`}>{p.agent}</span>}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

const STATUS_LABEL: Record<Status, string> = {
  run: "working", input: "needs you", idle: "ready", done: "done", shell: "shell",
};

function StatusDot({ status }: { status: Status }) {
  return <span className={`dot dot-${status}`} title={STATUS_LABEL[status]} />;
}

function AgentSelect({ agents, value, onChange }: { agents: AgentKind[]; value: string; onChange: (v: string) => void }) {
  return (
    <label>
      Agent
      <Select
        value={value}
        onChange={onChange}
        options={[
          ...agents.map((a) => ({ value: a.id, label: a.label, icon: <Logo agent={a.id} /> })),
          { value: "shell", label: "Plain shell", icon: <span className="sel-glyph">$</span> },
        ]}
      />
    </label>
  );
}

// useSubmit wraps a dialog's create action with busy/error state.
type Submit = { err: string | null; busy: boolean; run: () => Promise<void> };

function useSubmit(action: () => Promise<void>): Submit {
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const run = async () => {
    setBusy(true);
    setErr(null);
    try {
      await action();
    } catch (e) {
      setErr(errMsg(e));
    } finally {
      setBusy(false);
    }
  };
  return { err, busy, run };
}

function Modal({ title, hint, children, onClose, submit, submitLabel }: {
  title: string;
  hint?: ReactNode;
  children: ReactNode;
  onClose: () => void;
  submit: Submit;
  submitLabel: string;
}) {
  return (
    <div className="modal-scrim" onClick={onClose}>
      <form
        className="modal"
        onClick={(e) => e.stopPropagation()}
        onSubmit={(e) => {
          e.preventDefault();
          void submit.run();
        }}
      >
        <h2>{title}</h2>
        {hint && <p className="modal-hint">{hint}</p>}
        {children}
        {submit.err && <div className="chat-error">{submit.err}</div>}
        <div className="modal-actions">
          <button type="button" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" className="primary" disabled={submit.busy}>
            {submit.busy ? "…" : submitLabel}
          </button>
        </div>
      </form>
    </div>
  );
}

function RenameDialog({ what, label, onClose, onRename }: { what: "tab" | "workspace"; label: string; onClose: () => void; onRename: (label: string) => Promise<void> }) {
  const [value, setValue] = useState(label);
  const submit = useSubmit(() => onRename(value.trim()));
  return (
    <Modal title={what === "tab" ? "Rename tab" : "Rename workspace"} onClose={onClose} submit={submit} submitLabel="Rename">
      <label>
        Name
        <input autoFocus value={value} onChange={(e) => setValue(e.target.value)} onFocus={(e) => e.target.select()} />
      </label>
    </Modal>
  );
}

function ConfirmDialog({ text, label, onClose, onConfirm }: { text: string; label: string; onClose: () => void; onConfirm: () => void }) {
  return (
    <div className="modal-scrim" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()} role="alertdialog">
        <p className="confirm-text">{text}</p>
        <div className="modal-actions">
          <button type="button" onClick={onClose} autoFocus>
            Cancel
          </button>
          <button type="button" className="primary danger" onClick={onConfirm}>
            {label}
          </button>
        </div>
      </div>
    </div>
  );
}

function NewWorkspaceDialog({
  machines,
  agents,
  defaultMachine,
  onCreate,
  onClose,
}: {
  machines: Machine[];
  agents: AgentKind[];
  defaultMachine: string;
  onCreate: (machine: string, dir: string, name: string, agent: string) => Promise<void>;
  onClose: () => void;
}) {
  const [machine, setMachine] = useState(defaultMachine);
  const [dir, setDir] = useState("");
  const [name, setName] = useState("");
  const [agent, setAgent] = useState("claude");
  const submit = useSubmit(async () => {
    if (!dir.trim()) throw new Error("Project folder is required");
    await onCreate(machine, dir.trim(), name.trim(), agent);
  });
  const target = machines.find((m) => m.id === machine);
  return (
    <Modal title="New workspace" onClose={onClose} submit={submit} submitLabel="Create">
      {machines.length > 1 && (
        <label>
          Machine
          <Select
            value={machine}
            onChange={setMachine}
            options={machines.map((m) => ({
              value: m.id,
              label: m.label,
              hint: m.state !== "connected" ? m.state : m.host,
              disabled: m.state !== "connected",
              icon: <span className={`machine-state is-${m.state}`} />,
            }))}
          />
        </label>
      )}
      <label>
        Project folder{target && target.id !== "local" ? ` on ${target.label}` : ""}
        <input autoFocus value={dir} placeholder="/Users/you/Projects/app" onChange={(e) => setDir(e.target.value)} />
      </label>
      <label>
        Name (optional)
        <input value={name} placeholder="checkout-api" onChange={(e) => setName(e.target.value)} />
      </label>
      <AgentSelect agents={agents} value={agent} onChange={setAgent} />
    </Modal>
  );
}

function NewTabDialog({
  session,
  agents,
  onCreate,
  onClose,
}: {
  session: Session;
  agents: AgentKind[];
  onCreate: (name: string, agent: string) => Promise<void>;
  onClose: () => void;
}) {
  const [name, setName] = useState("");
  const [agent, setAgent] = useState("shell");
  const submit = useSubmit(() => onCreate(name.trim(), agent));
  return (
    <Modal
      title={`New tab in ${session.name}`}
      hint="Opens in the workspace's folder, like herdr's prefix+c."
      onClose={onClose}
      submit={submit}
      submitLabel="Create tab"
    >
      <label>
        Name (optional)
        <input autoFocus value={name} placeholder="tests" onChange={(e) => setName(e.target.value)} />
      </label>
      <AgentSelect agents={agents} value={agent} onChange={setAgent} />
    </Modal>
  );
}

function NewWorktreeDialog({
  session,
  agents,
  onCreate,
  onClose,
}: {
  session: Session;
  agents: AgentKind[];
  onCreate: (branch: string, base: string, name: string, agent: string) => Promise<void>;
  onClose: () => void;
}) {
  const [branch, setBranch] = useState(suggestBranch);
  const [base, setBase] = useState("");
  const [name, setName] = useState("");
  const [agent, setAgent] = useState("claude");
  const submit = useSubmit(async () => {
    if (!branch.trim()) throw new Error("Branch name is required");
    await onCreate(branch.trim(), base.trim(), name.trim(), agent);
  });
  const repo = session.worktree?.repo_name ?? session.name;
  return (
    <Modal
      title={`New worktree of ${repo}`}
      hint={
        <>
          <code>git worktree add</code> creates the branch's own checkout; it opens as a workspace
          nested under {repo}, and the agent starts there beside the first one.
        </>
      }
      onClose={onClose}
      submit={submit}
      submitLabel="Create worktree"
    >
      <label>
        Branch
        <input autoFocus value={branch} onChange={(e) => setBranch(e.target.value)} />
      </label>
      <label>
        Base (optional — branch, tag or commit; default: the current HEAD)
        <input value={base} placeholder="main" onChange={(e) => setBase(e.target.value)} />
      </label>
      <label>
        Workspace name (optional)
        <input value={name} placeholder="defaults to the branch" onChange={(e) => setName(e.target.value)} />
      </label>
      <AgentSelect agents={agents} value={agent} onChange={setAgent} />
    </Modal>
  );
}

function AddMachineDialog({
  onAdd,
  onClose,
}: {
  onAdd: (label: string, host: string) => Promise<void>;
  onClose: () => void;
}) {
  const [host, setHost] = useState("");
  const [label, setLabel] = useState("");
  const submit = useSubmit(async () => {
    if (!host.trim()) throw new Error("SSH host is required");
    await onAdd(label.trim(), host.trim());
  });
  return (
    <Modal
      title="Add a remote PC"
      hint={
        <>
          Machines saved in herdr appear on their own. This adds another: any host your{" "}
          <code>ssh</code> reaches with keys, with herdr running there.
        </>
      }
      onClose={onClose}
      submit={submit}
      submitLabel="Add & connect"
    >
      <label>
        SSH host
        <input autoFocus value={host} placeholder="build-box" onChange={(e) => setHost(e.target.value)} />
      </label>
      <label>
        Label (optional)
        <input value={label} placeholder="Build box" onChange={(e) => setLabel(e.target.value)} />
      </label>
    </Modal>
  );
}
