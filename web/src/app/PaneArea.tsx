// The pane area: the tab strip (its tools slot the lens's one control and the View menu), the three lenses of the pane on
// screen with the files rail beside them, or a transcript on its own.
// A lens stays mounted once seen, scoped to the pane: it never mounts hidden, since a terminal opened
// unseen would attach at 80×24 and resize the shared pane. The split view (app/split.ts) shows the chat and the
// terminal side by side, the store's lens naming the focused side; the terminal is the same mounted lens either way.

import { useEffect, useRef, useState } from "react";
import { useMedia } from "@/lib/media";
import { isMac, isTouch } from "@/lib/prefs";
import { basename } from "@/lib/text";
import { closeTab, createTab, renameTab } from "@/features/roster/api";
import { offerReopen } from "@/features/reopen/offer";
import { NO_MACHINES, startOn } from "@/features/roster/server";
import { onWindows } from "@/features/roster/system";
import { useRoster } from "@/features/roster/useRoster";
import { useSharing } from "@/features/roster/useSharing";
import { ChatTools, useChatViewItems } from "@/features/chat/ChatTools";
import { FindTool } from "@/features/chat/FindBar";
import TabStrip from "@/features/roster/TabStrip";
import { closeTabAsk } from "@/features/roster/tabs";
import Chat from "@/features/chat/Chat";
import FilesRail from "@/features/chat/FilesRail";
import Review from "@/features/review/Review";
import { TerminalLens, TerminalTools, useTerminalViewItems } from "@/features/terminal/TerminalLens";
import { timelapseRows } from "@/features/terminal/timelapse/row";
import { Screen } from "@/features/terminal/Screen";
import { LimitNotice } from "@/features/usage/LimitNotice";
import { accountWindows } from "@/features/usage/limits";
import { Icon, Notice, PANELS, Resizer } from "@/ui";
import { WindowsPaths } from "@/ui/Markdown";
import { act } from "./act";
import { folderOf, type Located } from "./current";
import { actions, app, type Lens } from "./store";
import { keepsTerminal, lensOn } from "./paneLens";
import { SPLIT_QUERY, shownLenses, splitOn } from "./split";
import ViewMenu from "./ViewMenu";
import { t as tr } from "@/lib/i18n";

export default function PaneArea({ current }: { current: Located | null }) {
  const agent = current?.pane.agent ?? "";
  const paneKey = current ? `${current.machine.id}|${current.pane.id}` : "";
  // whether the pane on screen came on screen as a shell: an agent started at its prompt keeps its terminal in view
  const [opened, setOpened] = useState({ pane: "", shell: false });
  if (opened.pane !== paneKey) setOpened({ pane: paneKey, shell: !agent });
  const pinned = opened.pane === paneKey && keepsTerminal(opened.shell, agent);
  const asked = app.use((s) => s.lens);
  const lens = pinned ? "terminal" : lensOn(asked, agent); // a shell has its terminal alone
  const viewer = app.use((s) => s.viewer);
  const footprint = app.use((s) => s.footprint);
  const chatSession = app.use((s) => s.chatSession);
  const narrow = useMedia("(max-width: 920px)"); // the stylesheet hides the rail there: it must not poll unseen
  const splitAsked = app.use((s) => s.split);
  const wide = useMedia(SPLIT_QUERY);
  const split = splitOn(splitAsked && !!agent, wide, isTouch, lens); // the chat beside the terminal; lens is then the focused side
  const { refresh, viewOnly, machines, server } = useRoster();
  const share = useSharing();
  const [revealed, setRevealed] = useState(""); // the pane shown anyway while sharing, until another is on screen
  const body = useRef<HTMLDivElement>(null);
  const viewerKey = viewer ? `viewer|${viewer.machine}|${viewer.session}` : "";
  const viewerHidden = !!viewer && (share.machineHidden(viewer.machine) || share.folderHidden(viewer.machine, viewer.dir));
  const covered = viewer ? viewerHidden && revealed !== viewerKey : !!current && share.sessionHidden(current.machine.id, current.session) && revealed !== paneKey;
  // the pin is said once to the store, so the header, the address and the palette agree; the lens is the person's again
  useEffect(() => {
    if (!pinned) return;
    actions.setLens("terminal");
    setOpened({ pane: paneKey, shell: false });
  }, [pinned, paneKey]);
  const [seenLenses, setSeenLenses] = useState<{ pane: string; lenses: Partial<Record<Lens, true>> }>({ pane: "", lenses: {} });
  useEffect(() => {
    if (!paneKey) return;
    setSeenLenses((m) => {
      const lenses = m.pane === paneKey ? { ...m.lenses } : {};
      let moved = m.pane !== paneKey;
      for (const l of shownLenses(lens, split)) {
        if (!lenses[l]) {
          lenses[l] = true;
          moved = true;
        }
      }
      return moved ? { pane: paneKey, lenses } : m;
    });
  }, [paneKey, lens, split]);
  // a covered pane's lenses take no focus, click or key: the veil is the only thing there
  useEffect(() => {
    const el = body.current;
    if (!el) return;
    for (const child of Array.from(el.children)) if (!child.classList.contains("sharing-veil")) child.toggleAttribute("inert", covered);
    if (covered && document.activeElement instanceof HTMLElement && el.contains(document.activeElement)) document.activeElement.blur();
  }, [covered, paneKey, lens, split, seenLenses]);
  const shown = (l: Lens) => shownLenses(lens, split).includes(l);
  const seen = (l: Lens) => shown(l) || (seenLenses.pane === paneKey && !!seenLenses.lenses[l]);
  // in the split a click or a focus in either side makes it the focused one, so the tools, the find and the keys follow
  const focusSide = (l: Lens) => {
    if (split && app.get().lens !== l) actions.setLens(l);
  };
  const place = viewer
    ? { machineId: viewer.machine, paneId: "", session: viewer.session, dir: viewer.dir, agent: viewer.subagent ? "" : "claude", workspace: basename(viewer.dir), machine: "" }
    : current
      ? { machineId: current.machine.id, paneId: current.pane.id, session: chatSession, dir: folderOf(current), agent: current.pane.agent, workspace: current.session.name, machine: current.machine.label, model: current.pane.usage?.name }
      : { machineId: "", paneId: "", session: null, dir: "", agent: "", workspace: "", machine: "" };
  const chatItems = useChatViewItems(place);
  const terminalItems = [...useTerminalViewItems(current?.pane ?? { cols: 0, rows: 0 } as Located["pane"], onWindows(current?.machine)), ...timelapseRows(place)];

  if (viewer) {
    const sub = viewer.subagent;
    return (
      <main className="pane-area">
        <Notice
          tone="info"
          icon={sub ? "sparkles" : "search"}
          className="viewer-bar"
          action={
            <>
              <ChatTools />
              <ViewMenu items={viewer.subagent ? chatItems.slice(0, 1) : chatItems} page={false} />
            </>
          }
          onDismiss={actions.closeViewer}
        >
          {sub ? (
            <>
              {tr("A subagent of")} <b>{share.redact(basename(viewer.dir))}</b>: {viewerHidden ? "" : sub.description} {tr("— its own conversation, read-only.")}
            </>
          ) : (
            <>
              {tr("A transcript from")} <b>{share.redact(basename(viewer.dir))}</b> {tr("— no agent runs there now, so it reads only")}
            </>
          )}
        </Notice>
        {covered ? (
          <SharingVeil onShow={() => setRevealed(viewerKey)} />
        ) : (
          <WindowsPaths.Provider value={onWindows(machines.find((m) => m.id === viewer.machine))}>
            <Chat
              key={`viewer|${viewer.machine}|${viewer.session}|${sub?.id ?? ""}`}
              machineId={viewer.machine}
              paneId=""
              status=""
              origin={sub ? `${sub.description} (a subagent of ${basename(viewer.dir)})` : `${basename(viewer.dir)} (an old transcript)`}
              dir={viewer.dir}
              session={viewer.session}
              subagent={sub?.id}
            />
          </WindowsPaths.Provider>
        )}
      </main>
    );
  }
  if (!current) {
    return (
      <main className="pane-area">
        <div className="placeholder">
          <p>{server && machines.length === 0 ? tr(NO_MACHINES) : tr("No pane selected.")}</p>
          {!viewOnly && machines.length > 0 && (
            <button type="button" className="btn" onClick={() => actions.openDialog({ kind: "workspace", machineId: startOn(machines) })}>
              {tr("Start an agent")}
            </button>
          )}
        </div>
      </main>
    );
  }
  const { machine, session, tab, pane } = current;
  const folder = folderOf(current);
  return (
    <main className="pane-area">
      <TabStrip
        session={session}
        tab={tab}
        pane={pane}
        readOnly={viewOnly}
        masked={share.sessionHidden(machine.id, session)}
        tools={
          <>
            {lens === "terminal" && !viewOnly && <FindTool what={tr("in the scrollback")} shortcut={isMac ? "⌘F" : "Ctrl+Shift+F"} />}
            {lens === "terminal" && !viewOnly && <TerminalTools />}
            {lens === "chat" && <ChatTools />}
            <ViewMenu items={lens === "terminal" && !viewOnly ? terminalItems : lens === "chat" ? chatItems : []} shell={!pane.agent} />
          </>
        }
        onSelect={(paneId) => actions.selectPane(machine.id, paneId)}
        onNewTab={() => actions.openDialog({ kind: "tab", machineId: machine.id, session })}
        onNewTerminal={() => void act(createTab({ machine: machine.id, workspace: session.id, name: "", agent: "" }).then((made) => actions.selectPane(machine.id, made.pane)))}
        onRename={(t, label) => void act(renameTab(machine.id, t.id, label))}
        onCloseTab={(t) => {
          const msg = closeTabAsk(session.tabs, t, share.sessionHidden(machine.id, session), share.redact(session.name));
          const agents = t.panes.filter((p) => p.agent).map((p) => p.id);
          actions.ask(msg, tr("Close tab"), () => void act(closeTab(machine.id, t.id)).then(() => offerReopen(machine.id, agents)));
        }}
      />
      <WindowsPaths.Provider value={onWindows(machine)}>
      <div className={`pane-body${split ? " is-split" : ""}${covered ? " is-covered" : ""}`} ref={body}>
        {covered && <SharingVeil onShow={() => setRevealed(paneKey)} />}
        {seen("changes") && (
          <div className="lens-pane" hidden={!shown("changes")}>
            <Review key={`${machine.id}|${session.id}|${folder}`} machineId={machine.id} paneId={pane.id} dir={folder} agent={pane.agent} onSeen={refresh} active={shown("changes")} editor={!onWindows(machine)} />
          </div>
        )}
        {seen("chat") && (
          <div className={`lens-pane${split ? " split-chat" : ""}`} hidden={!shown("chat")} onFocusCapture={() => focusSide("chat")} onPointerDownCapture={() => focusSide("chat")}>
            <Chat
              key={`${machine.id}|${pane.id}`}
              machineId={machine.id}
              paneId={pane.id}
              status={pane.status}
              origin={`${session.name}${machine.id !== "local" ? ` on ${machine.label}` : ""}`}
              dir={folder}
              usage={pane.usage}
              workspaceId={session.id}
              agent={pane.agent}
              active={shown("chat")}
              focused={lens === "chat"}
              viewOnly={viewOnly}
              notice={<LimitNotice machineId={machine.id} paneId={pane.id} agent={pane.agent} limit={pane.limit} windows={accountWindows(machines, Date.now())} viewOnly={viewOnly} />}
            />
          </div>
        )}
        {split && <Resizer spec={PANELS.split} edge="right" label={tr("Chat width in the split")} />}
        {seen("terminal") && viewOnly && (
          <div className={`lens-pane screen-lens${split ? " split-terminal" : ""}`} hidden={!shown("terminal")} onFocusCapture={() => focusSide("terminal")} onPointerDownCapture={() => focusSide("terminal")}>
            <Screen key={`${machine.id}|${pane.id}`} machineId={machine.id} paneId={pane.id} cols={pane.cols} busy={pane.status === "run" || pane.status === "input"} active={shown("terminal")} onOpen={() => {}} />
          </div>
        )}
        {seen("terminal") && !viewOnly && (
          <div className={`lens-pane${split ? " split-terminal" : ""}`} hidden={!shown("terminal")} onFocusCapture={() => focusSide("terminal")} onPointerDownCapture={() => focusSide("terminal")}>
            <TerminalLens key={`${machine.id}|${pane.id}`} machineId={machine.id} paneId={pane.id} cols={pane.cols} rows={pane.rows} agent={pane.agent !== ""} active={shown("terminal")} focused={lens === "terminal"} screen={onWindows(machine)} host={machine.host ?? machine.label} />
          </div>
        )}
        {footprint && pane.agent && <Resizer spec={PANELS.rail} edge="left" label={tr("Files rail width")} />}
        {footprint && pane.agent && (
          <FilesRail
            key={`${machine.id}|${pane.id}`}
            machineId={machine.id}
            paneId={pane.id}
            status={pane.status}
            dir={folder}
            active={!narrow}
            onOpenFile={(target) => actions.openFile(machine.id, pane.id, target, folder)}
            onOpenInCode={onWindows(machine) ? undefined : (path) => (viewOnly ? actions.openFile(machine.id, pane.id, { path }, folder) : actions.openInCode(machine.id, folder, path))} // a viewer has no Code mode: the file viewer reads it; a Windows PC has no editor
          />
        )}
      </div>
      </WindowsPaths.Provider>
    </main>
  );
}

// SharingVeil covers a hidden folder's pane while the screen is shared, until the person asks to see it
function SharingVeil({ onShow }: { onShow: () => void }) {
  return (
    <div className="sharing-veil">
      <Icon name="eye-off" size={18} />
      <p>{tr("Hidden while sharing")}</p>
      <button type="button" className="btn" onClick={onShow}>
        {tr("Show this pane")}
      </button>
    </div>
  );
}
