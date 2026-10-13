// The pane area: the tab strip and the three lenses of the pane on screen, or a transcript on its own.
// A lens stays mounted once seen, scoped to the pane: it never mounts hidden, since a terminal opened
// unseen would attach at 80×24 and resize the shared pane.

import { useEffect, useState } from "react";
import { basename } from "@/lib/text";
import { closeTab, renameTab } from "@/features/roster/api";
import { useRoster } from "@/features/roster/useRoster";
import TabStrip from "@/features/roster/TabStrip";
import Chat from "@/features/chat/Chat";
import Review from "@/features/review/Review";
import { TerminalLens, TerminalTools } from "@/features/terminal/TerminalLens";
import { Notice } from "@/ui";
import { act } from "./act";
import { folderOf, type Located } from "./current";
import { actions, app, type Lens } from "./store";

export default function PaneArea({ current }: { current: Located | null }) {
  const lens = app.use((s) => s.lens);
  const viewer = app.use((s) => s.viewer);
  const { refresh } = useRoster();
  const paneKey = current ? `${current.machine.id}|${current.pane.id}` : "";
  const [seenLenses, setSeenLenses] = useState<{ pane: string; lenses: Partial<Record<Lens, true>> }>({ pane: "", lenses: {} });
  useEffect(() => {
    if (!paneKey) return;
    setSeenLenses((m) => (m.pane === paneKey && m.lenses[lens] ? m : { pane: paneKey, lenses: { ...(m.pane === paneKey ? m.lenses : {}), [lens]: true } }));
  }, [paneKey, lens]);
  const seen = (l: Lens) => lens === l || (seenLenses.pane === paneKey && !!seenLenses.lenses[l]);

  if (viewer) {
    return (
      <main className="pane-area">
        <Notice tone="info" icon="search" className="viewer-bar" onDismiss={actions.closeViewer}>
          A transcript from <b>{basename(viewer.dir)}</b> — no agent runs there now, so it reads only.
        </Notice>
        <Chat key={`viewer|${viewer.machine}|${viewer.session}`} machineId={viewer.machine} paneId="" status="" origin={`${basename(viewer.dir)} (an old transcript)`} dir={viewer.dir} />
      </main>
    );
  }
  if (!current) {
    return (
      <main className="pane-area">
        <div className="placeholder">
          <p>No pane selected.</p>
          <button type="button" className="btn" onClick={() => actions.openDialog({ kind: "workspace", machineId: "local" })}>
            Start an agent
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
        tools={lens === "terminal" ? <TerminalTools pane={pane} /> : null}
        onSelect={(paneId) => actions.selectPane(machine.id, paneId)}
        onNewTab={() => actions.openDialog({ kind: "tab", machineId: machine.id, session })}
        onRename={(t, label) => void act(renameTab(machine.id, t.id, label))}
        onCloseTab={(t) => {
          const last = session.tabs.length === 1;
          const msg = last ? `Close the last tab? Workspace “${session.name}” closes with it.` : `Close tab “${t.label}”? Its ${t.panes.length} pane${t.panes.length === 1 ? "" : "s"} and agents stop.`;
          actions.ask(msg, "Close tab", () => void act(closeTab(machine.id, t.id)));
        }}
      />
      {seen("changes") && (
        <div className="lens-pane" hidden={lens !== "changes"}>
          <Review key={`${machine.id}|${session.id}|${folder}`} machineId={machine.id} paneId={pane.id} dir={folder} agent={pane.agent} onSeen={refresh} />
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
            active={lens === "chat"}
          />
        </div>
      )}
      {seen("terminal") && (
        <div className="lens-pane" hidden={lens !== "terminal"}>
          <TerminalLens key={`${machine.id}|${pane.id}`} machineId={machine.id} paneId={pane.id} cols={pane.cols} rows={pane.rows} agent={pane.agent !== ""} active={lens === "terminal"} />
        </div>
      )}
    </main>
  );
}
