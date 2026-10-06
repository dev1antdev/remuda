import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from "react";
import {
  addMachine,
  closeTab,
  createSession,
  createTab,
  createWorktree,
  fetchRoster,
  killSession,
  removeMachine,
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
import { hostOpensFile, installHostAPI, SELECT_FOLDER_EVENT } from "./host";
import KeyBar from "./KeyBar";
import { basename, errMsg, isTouch, readAlertPrefs, usePref } from "./prefs";
import { resyncPush } from "./push";
import TerminalView, { type Modifier, type SizeMode, type TermApi } from "./Terminal";

type Lens = "chat" | "terminal";
type Selection = { machineId: string; paneId: string };
type Located = { machine: Machine; session: Session; tab: Tab; pane: Pane };

// Every "make something" dialog, keyed so only one is open at a time.
type Dialog =
  | { kind: "workspace"; machineId: string }
  | { kind: "tab"; machineId: string; session: Session }
  | { kind: "worktree"; machineId: string; session: Session }
  | { kind: "machine" }
  | null;

type RowMenu = { machine: Machine; session: Session; x: number; y: number } | null;

// The pane a workspace row stands for: the focused pane of its active tab,
// else the first agent pane, else the first pane.
function representativePane(s: Session): { tab: Tab; pane: Pane } | null {
  const tab = s.tabs.find((t) => t.id === s.active_tab_id) ?? s.tabs[0];
  if (!tab) return null;
  const pane = tab.panes.find((p) => p.focused) ?? tab.panes.find((p) => p.agent) ?? tab.panes[0];
  return pane ? { tab, pane } : null;
}

// herdr packs a repository's worktree workspaces under the one on its main
// checkout; mirror that nesting.
function worktreeGroups(sessions: Session[]): Array<{ session: Session; children: Session[] }> {
  const parentByRepo = new Map<string, Session>();
  for (const s of sessions) {
    if (s.worktree && !s.worktree.linked && !parentByRepo.has(s.worktree.repo_key))
      parentByRepo.set(s.worktree.repo_key, s);
  }
  const childrenOf = new Map<string, Session[]>();
  const top: Session[] = [];
  for (const s of sessions) {
    const parent = s.worktree?.linked ? parentByRepo.get(s.worktree.repo_key) : undefined;
    if (parent && parent !== s) {
      childrenOf.set(parent.id, [...(childrenOf.get(parent.id) ?? []), s]);
    } else {
      top.push(s);
    }
  }
  return top.map((session) => ({ session, children: childrenOf.get(session.id) ?? [] }));
}

const ADJ = ["clear", "quiet", "amber", "brisk", "calm", "early", "fresh", "keen", "mild", "swift"];
const NOUN = ["forest", "river", "harbor", "meadow", "summit", "valley", "ridge", "grove", "lake", "field"];
function suggestBranch(): string {
  const pick = (a: string[]) => a[Math.floor(Math.random() * a.length)]!;
  return `worktree/${pick(ADJ)}-${pick(NOUN)}-${1000 + Math.floor(Math.random() * 9000)}`;
}

// What the URL asks to open: ?machine=&pane= (a tapped notification) or
// ?cwd= (an IDE embedding herdr-ui for the project it has open).
type Requested = Selection | { folder: string };

function requestedFromURL(): Requested | null {
  const q = new URLSearchParams(location.search);
  const machineId = q.get("machine");
  const paneId = q.get("pane");
  const cwd = q.get("cwd");
  if (machineId || paneId || cwd) history.replaceState(null, "", "/");
  if (machineId && paneId) return { machineId, paneId };
  if (cwd) return { folder: cwd.replace(/\/+$/, "") };
  return null;
}

function matches(x: Located, r: Requested): boolean {
  if ("folder" in r) return x.pane.cwd === r.folder || x.session.worktree?.checkout_path === r.folder;
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
  const [termFont, setTermFont] = usePref("termFont", isTouch ? 12 : 13);
  // machines folded shut in the sidebar, like herdr's collapsed groups
  const [collapsed, setCollapsed] = usePref<Record<string, boolean>>("collapsed", {});

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
    installHostAPI();
    void resyncPush(readAlertPrefs());
    const onFolder = (e: Event) => setRequested({ folder: (e as CustomEvent<string>).detail.replace(/\/+$/, "") });
    window.addEventListener(SELECT_FOLDER_EVENT, onFolder);
    if (!("serviceWorker" in navigator)) return () => window.removeEventListener(SELECT_FOLDER_EVENT, onFolder);
    const onMsg = (e: MessageEvent) => {
      const d = e.data as { type?: string; machine?: string; pane?: string };
      if (d?.type === "select-pane" && d.machine && d.pane) setRequested({ machineId: d.machine, paneId: d.pane });
    };
    navigator.serviceWorker.addEventListener("message", onMsg);
    return () => {
      window.removeEventListener(SELECT_FOLDER_EVENT, onFolder);
      navigator.serviceWorker.removeEventListener("message", onMsg);
    };
  }, []);

  // The row menu closes on any click elsewhere or Escape.
  useEffect(() => {
    if (!menu) return;
    const close = () => setMenu(null);
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && close();
    document.addEventListener("click", close);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("click", close);
      document.removeEventListener("keydown", onKey);
    };
  }, [menu]);

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

  const selectPane = useCallback((machineId: string, paneId: string) => {
    setRequested({ machineId, paneId });
    setDrawer(false);
  }, []);
  const selectSession = (m: Machine, s: Session) => {
    const rep = representativePane(s);
    if (rep) selectPane(m.id, rep.pane.id);
  };

  // stable for the memoized chat turns and the terminal's link provider
  const currentRef = useRef(current);
  currentRef.current = current;
  const openFile = useCallback((target: FileTarget) => {
    const c = currentRef.current;
    if (!c) return;
    if (hostOpensFile(c.machine.id, target)) return; // the IDE took it
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
    if (confirm(`Close “${s.name}”? ${n} pane${n === 1 ? "" : "s"} close and the agents in them stop.`))
      void killSession(m.id, s.id).then(refresh);
  };

  const workspaceRow = (m: Machine, s: Session, nested: boolean) => {
    const rep = representativePane(s);
    const selected = current?.machine.id === m.id && current.session.id === s.id;
    const folder = s.worktree?.checkout_path ?? rep?.pane.cwd ?? "";
    const agent = rep?.pane.agent ?? "";
    return (
      <div
        key={s.id}
        className={`ws-row${selected ? " sel" : ""}${nested ? " nested" : ""}`}
        role="button"
        tabIndex={0}
        onClick={() => selectSession(m, s)}
        onKeyDown={(e) => (e.key === "Enter" || e.key === " ") && selectSession(m, s)}
        title={folder}
      >
        <StatusDot status={s.status} />
        <span className="ws-copy">
          <span className="ws-label">{s.name}</span>
          <span className="ws-sub">
            {agent && <span className={`ws-agent agent-${agent}`}>{agent}</span>}
            {s.tabs.length > 1 && <span>{s.tabs.length} tabs</span>}
            <span className="ws-folder">{basename(folder)}</span>
          </span>
        </span>
        <button
          type="button"
          className="ws-menu-btn"
          title="Workspace actions"
          onClick={(e) => {
            e.stopPropagation();
            const r = e.currentTarget.getBoundingClientRect();
            setMenu(menu?.session.id === s.id ? null : { machine: m, session: s, x: r.right, y: r.bottom });
          }}
        >
          ⋯
        </button>
      </div>
    );
  };

  return (
    <div className="app">
      <header className="topbar">
        <button className="burger" onClick={() => setDrawer((d) => !d)}>
          ☰
        </button>
        <span className="title">
          {current ? (
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
        {current && (
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
                  {sizeMode === "keep" ? "⊡" : "⤢"}
                </button>
                <button
                  className={`kb-toggle${selectMode ? " on" : ""}`}
                  title={
                    selectMode
                      ? "Selection mode is on: drag selects text and copies it. Click to give the mouse back to the agent."
                      : "Selection mode: drag to select and copy text (otherwise hold Option/Alt while dragging)"
                  }
                  onClick={() => setSelectMode((v) => !v)}
                >
                  ⌖
                </button>
                <button className="kb-toggle" title="Smaller text" onClick={() => setTermFont((f) => Math.max(8, f - 1))}>
                  A−
                </button>
                <button className="kb-toggle" title="Larger text" onClick={() => setTermFont((f) => Math.min(20, f + 1))}>
                  A+
                </button>
                <button
                  className={`kb-toggle${keybar ? " on" : ""}`}
                  title="Key bar"
                  onClick={() => setKeybar((k) => !k)}
                >
                  ⌨
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
          🔔
        </button>
      </header>
      {alerts && (
        <>
          <div className="scrim clear" onClick={() => setAlerts(false)} />
          <AlertsPanel onClose={() => setAlerts(false)} />
        </>
      )}

      <div className="body">
        <aside className={drawer ? "sidebar open" : "sidebar"}>
          <div className="sidebar-head">
            <span>Machines</span>
            <button onClick={() => setDialog({ kind: "machine" })} title="Add a remote PC over SSH">
              + PC
            </button>
          </div>
          {machines.map((m) => (
            <div key={m.id} className={`machine${collapsed[m.id] ? " folded" : ""}`}>
              <div className="machine-head">
                <button
                  className="machine-toggle"
                  title={collapsed[m.id] ? "Show workspaces" : "Hide workspaces"}
                  onClick={() => setCollapsed((c) => ({ ...c, [m.id]: !c[m.id] }))}
                >
                  <span className={`machine-state is-${m.state}`} title={m.error || m.state} />
                  <span className="machine-label" title={m.host}>
                    {m.label}
                  </span>
                  <span className="machine-count">
                    {collapsed[m.id] && m.sessions.length > 0 ? m.sessions.length : ""}
                  </span>
                  <span className="chevron" aria-hidden="true">
                    {collapsed[m.id] ? "▸" : "▾"}
                  </span>
                </button>
                {m.source === "herdr" && (
                  <span className="machine-tag" title="Mirrored from `herdr machine list`">
                    herdr
                  </span>
                )}
                <button
                  className="icon-btn"
                  title={`New workspace on ${m.label}`}
                  disabled={m.state !== "connected"}
                  onClick={() => setDialog({ kind: "workspace", machineId: m.id })}
                >
                  +
                </button>
                {m.id !== "local" && m.source !== "herdr" && (
                  <button
                    className="icon-btn"
                    title="Remove machine (nothing on the PC is touched)"
                    onClick={() => {
                      if (confirm(`Remove machine “${m.label}”? Nothing on the PC is touched.`))
                        void removeMachine(m.id).then(refresh);
                    }}
                  >
                    ×
                  </button>
                )}
              </div>
              {m.state === "error" && <div className="machine-error">{m.error}</div>}
              {m.state === "connecting" && <div className="machine-error dim">connecting…</div>}
              {!collapsed[m.id] && (
                <div className="ws-tree">
                  {worktreeGroups(m.sessions).map(({ session, children }) => (
                    <div key={session.id} className="ws-group">
                      {workspaceRow(m, session, false)}
                      {children.length > 0 && (
                        <div className="ws-children">
                          {children.map((c) => workspaceRow(m, c, true))}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              )}
            </div>
          ))}
        </aside>
        {drawer && <div className="scrim" onClick={() => setDrawer(false)} />}

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
                onCloseTab={(t) => {
                  const last = current.session.tabs.length === 1;
                  const msg = last
                    ? `Close the last tab? Workspace “${current.session.name}” closes with it.`
                    : `Close tab “${t.label}”? Its ${t.panes.length} pane${t.panes.length === 1 ? "" : "s"} and agents stop.`;
                  if (confirm(msg)) void closeTab(current.machine.id, t.id).then(refresh);
                }}
              />
              {lens === "chat" ? (
                <Chat
                  key={`${current.machine.id}|${current.pane.id}`} // fresh state per pane: no stale session pick
                  machineId={current.machine.id}
                  paneId={current.pane.id}
                  status={current.pane.status}
                  onOpenFile={openFile}
                />
              ) : (
                <>
                  <TerminalView
                    machineId={current.machine.id}
                    paneId={current.pane.id}
                    cols={current.pane.cols}
                    rows={current.pane.rows}
                    mode={sizeMode}
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
          <button className="danger" onClick={() => { setMenu(null); closeWorkspace(menu.machine, menu.session); }}>
            Close workspace
          </button>
        </div>
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
}: {
  session: Session;
  tab: Tab;
  pane: Pane;
  onSelect: (paneId: string) => void;
  onNewTab: () => void;
  onCloseTab: (t: Tab) => void;
}) {
  return (
    <div className="tabstrip">
      <div className="tabs" role="tablist">
        {session.tabs.map((t) => {
          const first = t.panes.find((p) => p.focused) ?? t.panes[0];
          return (
            <div key={t.id} role="tab" aria-selected={t.id === tab.id} className={`tab${t.id === tab.id ? " sel" : ""}`}>
              <button className="tab-main" disabled={!first} onClick={() => first && onSelect(first.id)}>
                <StatusDot status={t.status} />
                <span className="tab-num">{t.number}</span>
                {t.named && <span className="tab-label">{t.label}</span>}
                {t.panes.length > 1 && <span className="tab-count">{t.panes.length}</span>}
              </button>
              <button className="tab-close" title="Close tab" onClick={() => onCloseTab(t)}>
                ×
              </button>
            </div>
          );
        })}
        <button className="tab-add" title="New tab in this workspace" onClick={onNewTab}>
          +
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
      <select value={value} onChange={(e) => onChange(e.target.value)}>
        {agents.map((a) => (
          <option key={a.id} value={a.id}>
            {a.label}
          </option>
        ))}
        <option value="shell">Plain shell</option>
      </select>
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
          <select value={machine} onChange={(e) => setMachine(e.target.value)}>
            {machines.map((m) => (
              <option key={m.id} value={m.id} disabled={m.state !== "connected"}>
                {m.label}
                {m.state !== "connected" ? ` (${m.state})` : ""}
              </option>
            ))}
          </select>
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
