// The pane area: the tab strip (its tools slot the lens's one control and the View menu), the three lenses of the pane on
// screen with the files rail beside them, or a transcript on its own.
// A lens stays mounted once seen, scoped to the pane: it never mounts hidden, since a terminal opened
// unseen would attach at 80×24 and resize the shared pane.

import { useEffect, useState } from "react";
import { useMedia } from "@/lib/media";
import { isMac } from "@/lib/prefs";
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
import { Notice, PANELS, Resizer } from "@/ui";
import { act } from "./act";
import { folderOf, type Located } from "./current";
import { actions, app, type Lens } from "./store";
import ViewMenu from "./ViewMenu";
import { tn, t as tr } from "@/lib/i18n";

export default function PaneArea({ current }: { current: Located | null }) {
  const lens = app.use((s) => s.lens);
  const viewer = app.use((s) => s.viewer);
  const footprint = app.use((s) => s.footprint);
  const chatSession = app.use((s) => s.chatSession);
  const narrow = useMedia("(max-width: 920px)"); // the stylesheet hides the rail there: it must not poll unseen
  const { refresh } = useRoster();
  const paneKey = current ? `${current.machine.id}|${current.pane.id}` : "";
  const [seenLenses, setSeenLenses] = useState<{ pane: string; lenses: Partial<Record<Lens, true>> }>({ pane: "", lenses: {} });
  useEffect(() => {
    if (!paneKey) return;
    setSeenLenses((m) => (m.pane === paneKey && m.lenses[lens] ? m : { pane: paneKey, lenses: { ...(m.pane === paneKey ? m.lenses : {}), [lens]: true } }));
  }, [paneKey, lens]);
  const seen = (l: Lens) => lens === l || (seenLenses.pane === paneKey && !!seenLenses.lenses[l]);
  const place = viewer
    ? { machineId: viewer.machine, paneId: "", session: viewer.session, dir: viewer.dir, agent: viewer.subagent ? "" : "claude", workspace: basename(viewer.dir), machine: "" }
    : current
      ? { machineId: current.machine.id, paneId: current.pane.id, session: chatSession, dir: folderOf(current), agent: current.pane.agent, workspace: current.session.name, machine: current.machine.label }
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
        tools={
          <>
            {lens === "terminal" && <FindTool what={tr("in the scrollback")} shortcut={isMac ? "⌘F" : "Ctrl+Shift+F"} />}
            {lens === "terminal" && <TerminalTools />}
            {lens === "chat" && <ChatTools />}
            <ViewMenu items={lens === "terminal" ? terminalItems : lens === "chat" ? chatItems : []} shell={!pane.agent} />
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
      <div className="pane-body">
        {seen("changes") && (
          <div className="lens-pane" hidden={lens !== "changes"}>
            <Review key={`${machine.id}|${session.id}|${folder}`} machineId={machine.id} paneId={pane.id} dir={folder} agent={pane.agent} onSeen={refresh} active={lens === "changes"} />
          </div>
        )}
        {seen("chat") && (
          <div className="lens-pane" hidden={lens !== "chat"}>
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
              active={lens === "chat"}
            />
          </div>
        )}
        {seen("terminal") && (
          <div className="lens-pane" hidden={lens !== "terminal"}>
            <TerminalLens key={`${machine.id}|${pane.id}`} machineId={machine.id} paneId={pane.id} cols={pane.cols} rows={pane.rows} agent={pane.agent !== ""} active={lens === "terminal"} />
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
