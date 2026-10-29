// The pane area: the tab strip (its tools slot the lens's one control and the View menu), the three lenses of the pane on
// screen with the files rail beside them, or a transcript on its own.
// A lens stays mounted once seen, scoped to the pane: it never mounts hidden, since a terminal opened
// unseen would attach at 80×24 and resize the shared pane. The split view (app/split.ts) shows the chat and the
// terminal side by side, the store's lens naming the focused side; the terminal is the same mounted lens either way.

import { useEffect, useState } from "react";
import { useMedia } from "@/lib/media";
import { isMac, isTouch } from "@/lib/prefs";
import { basename } from "@/lib/text";
import { closeTab, renameTab } from "@/features/roster/api";
import { useRoster } from "@/features/roster/useRoster";
import { ChatTools, useChatViewItems } from "@/features/chat/ChatTools";
import { FindTool } from "@/features/chat/FindBar";
import TabStrip from "@/features/roster/TabStrip";
import Chat from "@/features/chat/Chat";
import FilesRail from "@/features/chat/FilesRail";
import Review from "@/features/review/Review";
import { TerminalLens, TerminalTools, useTerminalViewItems } from "@/features/terminal/TerminalLens";
import { Screen } from "@/features/terminal/Screen";
import { Notice, PANELS, Resizer } from "@/ui";
import { act } from "./act";
import { folderOf, type Located } from "./current";
import { actions, app, type Lens } from "./store";
import { SPLIT_QUERY, shownLenses, splitOn } from "./split";
import ViewMenu from "./ViewMenu";
import { tn, t as tr } from "@/lib/i18n";

export default function PaneArea({ current }: { current: Located | null }) {
  const lens = app.use((s) => s.lens);
  const viewer = app.use((s) => s.viewer);
  const footprint = app.use((s) => s.footprint);
  const chatSession = app.use((s) => s.chatSession);
  const narrow = useMedia("(max-width: 920px)"); // the stylesheet hides the rail there: it must not poll unseen
  const splitAsked = app.use((s) => s.split);
  const wide = useMedia(SPLIT_QUERY);
  const split = splitOn(splitAsked, wide, isTouch, lens); // the chat beside the terminal; lens is then the focused side
  const { refresh, viewOnly } = useRoster();
  const paneKey = current ? `${current.machine.id}|${current.pane.id}` : "";
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
  const terminalItems = useTerminalViewItems(current?.pane ?? { cols: 0, rows: 0 } as Located["pane"]);

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
              A subagent of <b>{basename(viewer.dir)}</b>: {sub.description} — its own conversation, read-only.
            </>
          ) : (
            <>
              A transcript from <b>{basename(viewer.dir)}</b> {tr("— no agent runs there now, so it reads only")}
            </>
          )}
        </Notice>
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
      </main>
    );
  }
  if (!current) {
    return (
      <main className="pane-area">
        <div className="placeholder">
          <p>{tr("No pane selected.")}</p>
          <button type="button" className="btn" onClick={() => actions.openDialog({ kind: "workspace", machineId: "local" })}>
            {tr("Start an agent")}
          </button>
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
        onRename={(t, label) => void act(renameTab(machine.id, t.id, label))}
        onCloseTab={(t) => {
          const last = session.tabs.length === 1;
          const msg = last ? tr("Close the last tab? Workspace “{workspace}” closes with it.", { workspace: session.name }) : tn(t.panes.length, "Close tab “{tab}”? Its {n} pane and agent stop.", "Close tab “{tab}”? Its {n} panes and agents stop.", { tab: t.label });
          actions.ask(msg, tr("Close tab"), () => void act(closeTab(machine.id, t.id)));
        }}
      />
      <div className={`pane-body${split ? " is-split" : ""}`}>
        {seen("changes") && (
          <div className="lens-pane" hidden={!shown("changes")}>
            <Review key={`${machine.id}|${session.id}|${folder}`} machineId={machine.id} paneId={pane.id} dir={folder} agent={pane.agent} onSeen={refresh} active={shown("changes")} />
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
            <TerminalLens key={`${machine.id}|${pane.id}`} machineId={machine.id} paneId={pane.id} cols={pane.cols} rows={pane.rows} agent={pane.agent !== ""} active={shown("terminal")} focused={lens === "terminal"} />
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
            onOpenInCode={(path) => actions.openInCode(machine.id, folder, path)}
          />
        )}
      </div>
    </main>
  );
}
