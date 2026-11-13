// The files rail's Project tab: the workspace's folder as a tree, each folder read when opened, git's marks on every row,
// read again every ten seconds while on screen and soon after the agent's conversation says it did something. A row opens
// the file, and its menu opens it in the editor, shows its changes, copies its path or hands it to an agent.

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { actions } from "@/app/store";
import SendToAgent from "@/features/agents/SendToAgent";
import { useAgentTargets } from "@/features/agents/useAgentTargets";
import { submitPrompt } from "@/features/roster/api";
import type { OpenFile } from "@/features/files/target";
import { copyText } from "@/lib/host";
import { t } from "@/lib/i18n";
import { useLive, watchPane } from "@/lib/live";
import { pageVisible } from "@/lib/visibility";
import type { IconName, MenuItem } from "@/ui";
import TreeFolder, { type TreeContext } from "./TreeFolder";
import { absoluteOf, CHAT_PAUSE, fileMessage, REFRESH_EVERY, relativeOf, rowActions, type RowAction, type TreeEntry } from "./tree";

// the folders open in each tree, kept while the page stands so a tab switched away and back opens where it was
const opened = new Map<string, Set<string>>();

type Sending = { entry: TreeEntry; path: string; rel: string };

export default function ProjectTree({ machineId, paneId, dir, active, viewOnly, editor, onOpenFile }: {
  machineId: string;
  paneId: string;
  dir: string;
  active: boolean;
  viewOnly: boolean;
  editor: boolean; // the machine has a code-server for the Code mode
  onOpenFile: OpenFile;
}) {
  const key = `${machineId}|${dir}`;
  const [open, setOpen] = useState<Set<string>>(() => opened.get(key) ?? new Set());
  useEffect(() => setOpen(opened.get(key) ?? new Set()), [key]);
  const toggle = useCallback(
    (rel: string) =>
      setOpen((was) => {
        const next = new Set(was);
        if (!next.delete(rel)) next.add(rel);
        opened.set(key, next);
        return next;
      }),
    [key],
  );
  const tick = useTick(machineId, paneId, active);
  const [sending, setSending] = useState<Sending | null>(null);
  const agents = useAgentTargets();

  const run = useCallback(
    (action: RowAction, entry: TreeEntry, rel: string) => {
      const path = absoluteOf(dir, rel);
      const copy = (text: string) => void copyText(text).then((ok) => actions.notify(ok ? t("Copied {path}", { path: text }) : t("The path could not be copied"), { brief: true, error: !ok }));
      if (action === "open") onOpenFile({ path, prefer: entry.gone ? "changes" : "file" });
      else if (action === "changes") onOpenFile({ path, prefer: "changes" });
      else if (action === "editor") actions.openInCode(machineId, dir, path);
      else if (action === "copy-relative") copy(relativeOf(dir, rel));
      else if (action === "copy-full") copy(path);
      else setSending({ entry, path, rel });
    },
    [dir, machineId, onOpenFile],
  );
  const ctx = useMemo<TreeContext>(
    () => ({
      machineId,
      dir,
      active,
      tick,
      isOpen: (rel) => open.has(rel),
      toggle,
      open: (entry, rel) => run("open", entry, rel),
      items: (entry, rel) => menuItems(rowActions(entry, { viewOnly, editor }), (action) => run(action, entry, rel)),
    }),
    [machineId, dir, active, tick, open, toggle, run, viewOnly, editor],
  );

  return (
    <div className="footprint-list tree">
      <TreeFolder rel="" depth={0} ctx={ctx} />
      {sending && (
        <SendToAgent
          title={sending.entry.dir ? t("Send a folder to an agent") : t("Send a file to an agent")}
          text=""
          ask={t("What should it do?")}
          about={
            <>
              {sending.entry.dir ? t("Folder") : t("File")}: <code>{sending.path}</code>
            </>
          }
          compose={(instruction) => fileMessage(instruction, sending.path, !!sending.entry.dir)}
          agents={agents}
          currentPane={paneId ? { machineId, paneId } : null}
          rememberKey={`file:${machineId}|${paneId}`}
          onClose={() => setSending(null)}
          onSend={async (target, text) => {
            await submitPrompt(target.machineId, target.paneId, text);
            setSending(null);
            actions.notify(t("Sent to {agent} in {workspace}: {name}", { name: sending.entry.name, agent: target.agent, workspace: target.sessionName }), { brief: true });
          }}
        />
      )}
    </div>
  );
}

const ACTION_ROWS: Record<RowAction, { label: () => string; icon: IconName }> = {
  open: { label: () => t("Open"), icon: "file" },
  editor: { label: () => t("Open in the editor"), icon: "code" },
  changes: { label: () => t("Show changes"), icon: "git-branch" },
  "copy-relative": { label: () => t("Copy relative path"), icon: "copy" },
  "copy-full": { label: () => t("Copy full path"), icon: "copy" },
  send: { label: () => t("Send to agent…"), icon: "send" },
};

// menuItems turns a row's actions into its menu, the copies and the hand-off each set apart
function menuItems(rowActions: RowAction[], run: (action: RowAction) => void): MenuItem[] {
  const items: MenuItem[] = [];
  let group = "";
  for (const action of rowActions) {
    const kind = action.startsWith("copy") ? "copy" : action === "send" ? "send" : "open";
    if (group && kind !== group) items.push({ sep: true });
    group = kind;
    items.push({ label: ACTION_ROWS[action].label(), icon: ACTION_ROWS[action].icon, onSelect: () => run(action) });
  }
  return items;
}

// useTick moves every ten seconds while the tree is on screen and the page visible, and a moment after the pane's
// conversation moved, at most every few seconds however busy the agent is
function useTick(machineId: string, paneId: string, active: boolean): number {
  const [tick, setTick] = useState(0);
  const last = useRef(0);
  const pending = useRef<ReturnType<typeof setTimeout> | null>(null);
  const bump = useCallback(() => {
    if (pending.current) return;
    const wait = Math.max(0, last.current + CHAT_PAUSE - Date.now());
    pending.current = setTimeout(() => {
      pending.current = null;
      last.current = Date.now();
      setTick((n) => n + 1);
    }, wait);
  }, []);
  useEffect(() => {
    if (!active) return;
    const timer = setInterval(() => pageVisible() && bump(), REFRESH_EVERY);
    return () => clearInterval(timer);
  }, [active, bump]);
  useEffect(
    () => () => {
      if (pending.current) clearTimeout(pending.current);
    },
    [],
  );
  useEffect(() => {
    if (!active || !paneId) return;
    return watchPane(machineId, paneId);
  }, [active, machineId, paneId]);
  useLive("chat", (event) => {
    if (active && paneId && event.machine === machineId && event.pane === paneId) bump();
  });
  return tick;
}
