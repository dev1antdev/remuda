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
  type Tab, type SearchHit, encodePath } from "./api";
import AlertsPanel from "./AlertsPanel";
import Chat from "./Chat";
import FileViewer, { type FileTarget } from "./FileViewer";
import KeyBar from "./KeyBar";
import { basename, errMsg, isTouch, readAlertPrefs, usePref } from "./prefs";
import { machineLabel, machineOptions } from "./options";
import { Dialog, Notice, StatusDot, useSubmit } from "./ui";
import { resyncPush } from "./push";
import TerminalView, { type Modifier, type SizeMode, type TermApi } from "./Terminal";
import Tasks, { type AgentTarget } from "./Tasks";
import Code from "./Code";
import Review from "./Review";
import Schedules from "./Schedules";
import Search from "./Search";
import Usage, { fillClass, tok, usd } from "./Usage";
import Sidebar from "./Sidebar";
import Select from "./Select";
import Logo from "./Logo";
import { DEFAULT_RADAR, brandColor, type RadarPrefs } from "./radar";
import Icon, { type IconName } from "./icons";

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
  const paneId = q.get("pane");
  if (!paneId) return null;
  return { machineId: q.get("machine") || "local", paneId };
}

// the agent a "make something" dialog offers first: claude when herdr knows
// it, else the first one herdr lists
function defaultAgent(agents: AgentKind[], want: string): string {
  return agents.some((a) => a.id === want) ? want : (agents[0]?.id ?? "shell");
}

type NoticeMsg = { text: string; error?: boolean };

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
  const [notice, setNotice] = useState<NoticeMsg | null>(null);
  // an in-app confirmation: window.confirm() is unreliable inside embedded
  // browsers, and a dialog of our own matches the rest
  const [confirmReq, setConfirmReq] = useState<{ text: string; label: string; run: () => void } | null>(null);
  const ask = (text: string, label: string, run: () => void) => setConfirmReq({ text, label, run });
  const [termApi, setTermApi] = useState<TermApi | null>(null);
  const [modifier, setModifier] = useState<Modifier>(null);
  // selection mode: a plain drag selects text instead of reaching the agent
  const [selectMode, setSelectMode] = useState(false);
  const copied = useCallback((n: number) => setNotice({ text: `Copied ${n} character${n === 1 ? "" : "s"}` }), []);
  const [file, setFile] = useState<{ machineId: string; paneId: string; target: FileTarget } | null>(null);
  const [keybar, setKeybar] = usePref("keybar", isTouch);
  // A phone keeps the pane's own grid by default: resizing the shared pane
  // to a phone would squeeze the desktop's view of it too.
  const [sizeMode, setSizeMode] = usePref<SizeMode>("sizeMode", isTouch ? "keep" : "fit");
  // what the window is for right now: herdr's sessions, or the task list
  const [mode, setMode] = usePref<"herdr" | "tasks" | "code">("mode", "herdr");
  // the editor stays mounted once opened, so coming back does not reload it
  const [codeOpened, setCodeOpened] = useState(() => mode === "code");
  const [schedules, setSchedules] = useState(false);
  const [usagePanel, setUsagePanel] = useState(false);
  useEffect(() => {
    if (mode === "code") setCodeOpened(true);
  }, [mode]);
  // search across every transcript; a hit opens in the pane living in that
  // folder, or — none there — as a transcript on its own, read-only
  const [search, setSearch] = useState(false);
  const [viewer, setViewer] = useState<SearchHit | null>(null);
  // the jump names its pane ("" = the viewer): a Chat gets it only when it is that pane's
  const [jump, setJump] = useState<{ machineId: string; paneId: string; session: string; turn: string; n: number } | null>(null);
  const jumps = useRef(0);
  const jumpFor = (machineId: string, paneId: string) => (jump && jump.machineId === machineId && jump.paneId === paneId ? jump : null);
  useEffect(() => {
    // ⌘K on a Mac, Ctrl+K elsewhere — by key code, whatever the layout; a
    // terminal keeps its own Ctrl+K
    const mac = /Mac|iPhone|iPad/.test(navigator.platform);
    const onKey = (e: KeyboardEvent) => {
      if (e.code !== "KeyK" || e.altKey || e.shiftKey || (mac ? !e.metaKey : !e.ctrlKey)) return;
      if (!mac && (e.target as Element | null)?.closest?.(".xterm")) return;
      e.preventDefault();
      setSearch((o) => !o);
    };
    window.addEventListener("keydown", onKey, true);
    return () => window.removeEventListener("keydown", onKey, true);
  }, []);
  // a pane asked for from outside (a tapped notification, a deep link) is
  // an Agents-mode thing, whatever the device was left in
  const requestPane = (r: Requested) => {
    setRequested(r);
    setViewer(null);
    setMode("herdr");
  };
  // …including the one the page was opened with; the query goes, so a
  // reload does not ask for it again
  useEffect(() => {
    if (!requested) return;
    setMode("herdr");
    history.replaceState(null, "", "/");
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

  const refreshSeq = useRef(0);
  const refresh = useCallback(async () => {
    const seq = ++refreshSeq.current;
    try {
      const res = await fetchRoster(rosterEtag.current);
      if (seq !== refreshSeq.current) return; // a later poll already answered
      if (res) {
        rosterEtag.current = res.etag;
        setMachines(res.roster.machines);
        setAgents(res.roster.agents);
      }
      setOffline(false);
    } catch {
      if (seq === refreshSeq.current) setOffline(true);
    }
  }, []);

  // a roster change that may fail: refresh on success, say why on failure
  const act = useCallback((p: Promise<unknown>) => p.then(refresh, (e) => setNotice({ text: errMsg(e), error: true })), [refresh]);

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
  // already open tab arrives as a message from the service worker
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
    const t = setTimeout(() => setNotice(null), notice.text.startsWith("Copied") ? 2000 : 10000);
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
    () => (requested && allPanes.find((x) => matches(x, requested))) ?? allPanes.find((x) => x.pane.agent) ?? allPanes[0] ?? null,
    [allPanes, requested],
  );

  const openHit = (hit: SearchHit) => {
    setSearch(false);
    // the pane whose folder is the hit's project (a cd inside the session
    // does not move the transcript)
    const there = allPanes.filter((x) => x.machine.id === hit.machine && (encodePath(x.session.dir ?? "") === hit.project || encodePath(x.pane.cwd) === hit.project));
    const pick = there.find((x) => x.pane.agent === "claude") ?? there.find((x) => x.pane.agent) ?? there[0];
    setJump({ machineId: hit.machine, paneId: pick ? pick.pane.id : "", session: hit.session, turn: hit.uuid, n: ++jumps.current });
    setLens("chat");
    if (pick) {
      setViewer(null);
      requestPane({ machineId: pick.machine.id, paneId: pick.pane.id });
    } else {
      setViewer(hit);
      setMode("herdr");
    }
  };

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
    if (mode !== "herdr" || viewer || !current || seenState !== "done") return;
    const look = () => {
      if (document.visibilityState === "visible") void markSeen(current.machine.id, current.pane.id).catch(() => {});
    };
    look();
    document.addEventListener("visibilitychange", look);
    return () => document.removeEventListener("visibilitychange", look);
  }, [mode, !!viewer, current?.machine.id, current?.pane.id, seenState]); // eslint-disable-line react-hooks/exhaustive-deps

  const selectPane = useCallback((machineId: string, paneId: string) => {
    setRequested({ machineId, paneId });
    setViewer(null);
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
    if (made.warning) setNotice({ text: made.warning });
  };

  const closeWorkspace = (m: Machine, s: Session) => {
    const n = s.tabs.reduce((a, t) => a + t.panes.length, 0);
    ask(`Close “${s.name}”? ${n} pane${n === 1 ? "" : "s"} close and the agents in them stop.`, "Close workspace", () => void act(killSession(m.id, s.id)));
  };

  return (
    <div className="app">
      <header className="topbar">
        <button type="button" className="burger icon-btn" onClick={() => setDrawer((d) => !d)} title="Menu" aria-expanded={drawer}>
          <Icon name="menu" size={18} />
        </button>
        <nav className="mode-switch">
          <button className={mode === "herdr" ? "on" : ""} onClick={() => setMode("herdr")} title="Your agents and terminals, from herdr">
            Agents
          </button>
          <button className={mode === "tasks" ? "on" : ""} onClick={() => setMode("tasks")} title="Your tasks, kept on this herdr-ui">
            Tasks
          </button>
          <button className={mode === "code" ? "on" : ""} onClick={() => setMode("code")} title="VS Code on a machine, through herdr-ui">
            Code
          </button>
        </nav>
        <span className="title">
          {mode !== "herdr" ? null : viewer ? (
            <>
              {viewer.machine !== "local" && <span className="title-machine">{machineLabel(machines, viewer.machine)} ›</span>}
              {basename(viewer.dir)}
              <span className="title-agent"> · transcript</span>
            </>
          ) : current ? (
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
        {mode === "herdr" && current && !viewer && (
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
            {current.pane.usage && current.pane.usage.window > 0 && (
              <button
                type="button"
                className={`usage-pill${fillClass(current.pane.usage.context / current.pane.usage.window)}`}
                title={`${current.pane.usage.name}: context ${tok(current.pane.usage.context)} of ${tok(current.pane.usage.window)} (${Math.round((100 * current.pane.usage.context) / current.pane.usage.window)}%) · ${current.pane.usage.turns} responses · ${usd(current.pane.usage.cost)} API-equivalent so far. Click for the usage report.`}
                onClick={() => setUsagePanel(true)}
              >
                <span className="usage-pill-bar">
                  <span style={{ width: `${Math.min(100, (100 * current.pane.usage.context) / current.pane.usage.window)}%` }} />
                </span>
                {Math.round((100 * current.pane.usage.context) / current.pane.usage.window)}%
                <span className="usage-pill-cost"> · {usd(current.pane.usage.cost)}</span>
              </button>
            )}
            {lens === "terminal" && (
              <>
                <span className="lens-sep" />
                <button
                  className={`icon-btn kb-toggle${sizeMode === "keep" ? " on" : ""}`}
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
                    className={`icon-btn kb-toggle${minCols ? " on" : ""}`}
                    aria-expanded={!!widthMenu}
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
                  className={`icon-btn kb-toggle${selectMode ? " on" : ""}`}
                  title={
                    selectMode
                      ? "Selection mode is on: drag selects text and copies it. Click to give the mouse back to the agent."
                      : "Selection mode: drag to select and copy text (otherwise hold Option/Alt while dragging)"
                  }
                  onClick={() => setSelectMode((v) => !v)}
                >
                  <Icon name="text-select" />
                </button>
                <button className="icon-btn kb-toggle kb-font" title="Smaller text" onClick={() => setTermFont((f) => Math.max(8, f - 1))}>
                  <Icon name="text-smaller" />
                </button>
                <button className="icon-btn kb-toggle kb-font" title="Larger text" onClick={() => setTermFont((f) => Math.min(20, f + 1))}>
                  <Icon name="text-larger" />
                </button>
                <button
                  className={`icon-btn kb-toggle${keybar ? " on" : ""}`}
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
          <button type="button" className={`pill ${notice.error ? "pill-off" : "pill-warn"}`} title={`${notice.text} — click to dismiss`} onClick={() => setNotice(null)}>
            {notice.text}
          </button>
        )}
        <button type="button" className="icon-btn bell" title="Search every conversation (⌘K / Ctrl+K)" onClick={() => setSearch(true)}>
          <Icon name="search" size={18} />
        </button>
        <button type="button" className={`icon-btn bell${alerts ? " on" : ""}`} title="Alerts on this device" aria-expanded={alerts} onClick={() => setAlerts((a) => !a)}>
          <Icon name="bell" size={18} />
        </button>
      </header>
      {alerts && <AlertsPanel onClose={() => setAlerts(false)} />}

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
            machines={machines}
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
            onRemoveMachine={(m) => ask(`Remove machine “${m.label}”? Nothing on the PC is touched.`, "Remove", () => void act(removeMachine(m.id)))}
            onAddMachine={() => setDialog({ kind: "machine" })}
            onPrefs={setRadarPrefs}
            onSchedules={() => setSchedules(true)}
            onUsage={() => setUsagePanel(true)}
          />
        </aside>
        )}
        {mode === "herdr" && drawer && <div className="scrim" onClick={() => setDrawer(false)} />}

        {mode === "herdr" && (
        <main className="pane-area">
          {viewer ? (
            <>
              <Notice tone="info" icon="search" className="viewer-bar" onDismiss={() => setViewer(null)}>
                A transcript from <b>{basename(viewer.dir)}</b> — no agent runs there now, so it reads only.
              </Notice>
              <Chat
                key={`viewer|${viewer.machine}|${viewer.session}`}
                machineId={viewer.machine}
                paneId=""
                status=""
                onOpenFile={openFile}
                agents={agentTargets}
                origin={`${basename(viewer.dir)} (an old transcript)`}
                dir={viewer.dir}
                jump={jumpFor(viewer.machine, "")}
              />
            </>
          ) : current ? (
            <>
              <TabStrip
                session={current.session}
                tab={current.tab}
                pane={current.pane}
                onSelect={(paneId) => selectPane(current.machine.id, paneId)}
                onNewTab={() =>
                  setDialog({ kind: "tab", machineId: current.machine.id, session: current.session })
                }
                onRename={(t, label) => void act(renameTab(current.machine.id, t.id, label))}
                onCloseTab={(t) => {
                  const last = current.session.tabs.length === 1;
                  const msg = last
                    ? `Close the last tab? Workspace “${current.session.name}” closes with it.`
                    : `Close tab “${t.label}”? Its ${t.panes.length} pane${t.panes.length === 1 ? "" : "s"} and agents stop.`;
                  ask(msg, "Close tab", () => void act(closeTab(current.machine.id, t.id)));
                }}
              />
              {lens === "changes" ? (
                <Review
                  key={`${current.machine.id}|${current.session.id}|${current.session.dir || current.pane.cwd}`} // the draft is per folder
                  machineId={current.machine.id}
                  paneId={current.pane.id}
                  dir={current.session.dir || current.pane.cwd}
                  agent={current.pane.agent}
                  agents={agentTargets}
                  currentPane={{ machineId: current.machine.id, paneId: current.pane.id }}
                  onOpenFile={(path, line) => openFile({ path, line })}
                  onSeen={() => void refresh()}
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
                  jump={jumpFor(current.machine.id, current.pane.id)}
                  usage={current.pane.usage}
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
              <button type="button" className="btn" onClick={() => setDialog({ kind: "workspace", machineId: "local" })}>
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
        <div className="row-menu" role="menu" style={{ left: menu.x, top: menu.y }} onClick={(e) => e.stopPropagation()}>
          <button type="button" role="menuitem" onClick={() => { setMenu(null); setDialog({ kind: "tab", machineId: menu.machine.id, session: menu.session }); }}>
            New tab…
          </button>
          <button type="button" role="menuitem" onClick={() => { setMenu(null); setDialog({ kind: "worktree", machineId: menu.machine.id, session: menu.session }); }}>
            New worktree…
          </button>
          <button type="button" role="menuitem" onClick={() => { setMenu(null); setDialog({ kind: "rename", machineId: menu.machine.id, what: "workspace", id: menu.session.id, label: menu.session.name }); }}>
            Rename workspace…
          </button>
          {(() => {
            const t = menu.session.tabs.find((x) => x.id === menu.session.active_tab_id) ?? menu.session.tabs[0];
            return t ? (
              <button type="button" role="menuitem" onClick={() => { setMenu(null); setDialog({ kind: "rename", machineId: menu.machine.id, what: "tab", id: t.id, label: t.named ? t.label : "" }); }}>
                Rename tab {t.named ? `“${t.label}”` : t.number}…
              </button>
            ) : null;
          })()}
          <button type="button" role="menuitem" className="danger" onClick={() => { setMenu(null); closeWorkspace(menu.machine, menu.session); }}>
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
        <div className="row-menu width-menu" role="menu" style={{ left: widthMenu.x, top: widthMenu.y }} onClick={(e) => e.stopPropagation()}>
          {MIN_WIDTHS.map((n) => (
            <button
              key={n}
              type="button"
              role="menuitemradio"
              aria-checked={n === minCols}
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
      {search && <Search machines={machines} onClose={() => setSearch(false)} onOpen={openHit} />}
      {usagePanel && (
        <Usage
          machines={machines}
          onClose={() => setUsagePanel(false)}
          onOpenSession={(machine, project, session, dir) => {
            setUsagePanel(false);
            openHit({ machine, project, session, dir, uuid: "", ts: "", role: "assistant", snippet: "" });
          }}
        />
      )}

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
              <div key={t.id} className="tab sel editing">
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
            <div key={t.id} className={`tab${t.id === tab.id ? " sel" : ""}`}>
              <button
                type="button"
                role="tab"
                aria-selected={t.id === tab.id}
                className="tab-main"
                disabled={!first}
                title={t.named ? `${t.label} — double-click to rename` : "Double-click to name this tab"}
                onClick={() => first && onSelect(first.id)}
                onDoubleClick={() => setEditing({ id: t.id, value: t.named ? t.label : "" })}
              >
                <StatusDot status={t.status} title={STATUS_LABEL[t.status]} />
                <span className="tab-num">{t.number}</span>
                {t.named && <span>{t.label}</span>}
                {t.panes.length > 1 && <span className="tab-count">{t.panes.length}</span>}
              </button>
              <button type="button" className="icon-btn tab-close" title="Close tab" aria-label={`Close tab ${t.named ? t.label : t.number}`} onClick={() => onCloseTab(t)}>
                <Icon name="x" size={12} />
              </button>
            </div>
          );
        })}
        <button type="button" className="icon-btn tab-add" title="New tab in this workspace" onClick={onNewTab}>
          <Icon name="plus" size={14} />
        </button>
      </div>
      {tab.panes.length > 1 && (
        <div className="split-panes">
          {tab.panes.map((p, i) => (
            <button
              key={p.id}
              type="button"
              className={`split-pane${p.id === pane.id ? " sel" : ""}`}
              aria-pressed={p.id === pane.id}
              onClick={() => onSelect(p.id)}
              title={p.cwd}
            >
              <span className="tree-glyph">{i === tab.panes.length - 1 ? "└" : "├"}</span>
              <StatusDot status={p.status} title={STATUS_LABEL[p.status]} />
              <span>{p.title}</span>
              {p.agent && <span className="ws-agent" style={{ color: brandColor(p.agent) }}>{p.agent}</span>}
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

function AgentSelect({ agents, value, onChange }: { agents: AgentKind[]; value: string; onChange: (v: string) => void }) {
  return (
    <label>
      Agent
      <Select
        value={value}
        onChange={onChange}
        options={[
          ...agents.map((a) => ({ value: a.id, label: a.label, icon: <Logo agent={a.id} /> })),
          { value: "shell", label: "Plain shell", icon: <Icon name="terminal" size={14} /> },
        ]}
      />
    </label>
  );
}

// A dialog that is a form: fields, an error if the action failed, Cancel
// and the one filled button.
type Submit = { err: string; busy: boolean; run: () => Promise<void> };

function FormDialog({ title, icon, hint, children, onClose, submit, submitLabel }: {
  title: string;
  icon?: IconName;
  hint?: ReactNode;
  children: ReactNode;
  onClose: () => void;
  submit: Submit;
  submitLabel: string;
}) {
  return (
    <Dialog
      as="form"
      title={title}
      icon={icon}
      onClose={onClose}
      onSubmit={(e) => {
        e.preventDefault();
        void submit.run();
      }}
    >
      {hint && <p className="modal-hint">{hint}</p>}
      {children}
      {submit.err && <Notice tone="error">{submit.err}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          Cancel
        </button>
        <button type="submit" className="btn primary" disabled={submit.busy}>
          {submit.busy ? "…" : submitLabel}
        </button>
      </div>
    </Dialog>
  );
}

function RenameDialog({ what, label, onClose, onRename }: { what: "tab" | "workspace"; label: string; onClose: () => void; onRename: (label: string) => Promise<void> }) {
  const [value, setValue] = useState(label);
  const submit = useSubmit(() => onRename(value.trim()));
  return (
    <FormDialog title={what === "tab" ? "Rename tab" : "Rename workspace"} icon="edit" onClose={onClose} submit={submit} submitLabel="Rename">
      <label>
        Name
        <input autoFocus value={value} onChange={(e) => setValue(e.target.value)} onFocus={(e) => e.target.select()} />
      </label>
    </FormDialog>
  );
}

// a yes/no before something destructive; focus starts on Cancel
function ConfirmDialog({ text, label, onClose, onConfirm }: { text: string; label: string; onClose: () => void; onConfirm: () => void }) {
  return (
    <Dialog onClose={onClose}>
      <p className="confirm-text">{text}</p>
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          Cancel
        </button>
        <button type="button" className="btn danger" onClick={onConfirm}>
          {label}
        </button>
      </div>
    </Dialog>
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
  const [agent, setAgent] = useState(() => defaultAgent(agents, "claude"));
  const submit = useSubmit(async () => {
    if (!dir.trim()) throw new Error("Project folder is required");
    await onCreate(machine, dir.trim(), name.trim(), agent);
  });
  const target = machines.find((m) => m.id === machine);
  // every machine is listed; one that is not connected cannot be picked
  const options = machineOptions(machines, { onlyConnected: false }).map((o) => {
    const m = machines.find((x) => x.id === o.value);
    return m && m.state !== "connected" ? { ...o, hint: m.state, disabled: true } : { ...o, hint: m?.host };
  });
  return (
    <FormDialog title="New workspace" icon="folder" onClose={onClose} submit={submit} submitLabel="Create">
      {machines.length > 1 && (
        <label>
          Machine
          <Select value={machine} onChange={setMachine} options={options} />
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
    </FormDialog>
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
    <FormDialog
      title={`New tab in ${session.name}`}
      icon="plus"
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
    </FormDialog>
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
  const [agent, setAgent] = useState(() => defaultAgent(agents, "claude"));
  const submit = useSubmit(async () => {
    if (!branch.trim()) throw new Error("Branch name is required");
    await onCreate(branch.trim(), base.trim(), name.trim(), agent);
  });
  const repo = session.worktree?.repo_name ?? session.name;
  return (
    <FormDialog
      title={`New worktree of ${repo}`}
      icon="git-branch"
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
    </FormDialog>
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
    <FormDialog
      title="Add a remote PC"
      icon="monitor"
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
    </FormDialog>
  );
}
