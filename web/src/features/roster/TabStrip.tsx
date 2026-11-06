// TabStrip is the workspace's tabs above the pane: each says what it runs — the agent by its logo and name, a shell as
// "terminal" — and + opens a terminal in the workspace's folder at once, or an agent through its dialog; a split tab
// lists its panes under the row. A lens's own controls go in the right slot (tools), never in the header.

import { useState, type ReactNode } from "react";
import { Icon, Logo, Menu, StatusDot, Tip } from "@/ui";
import { VENDOR_NAME, brandColor } from "@/ui/brand";
import type { Pane, Session, Status, Tab } from "./api";
import { tabFaces } from "./tabs";
import { t as tr } from "@/lib/i18n";

const STATUS_LABEL: Record<Status, string> = {
  run: tr("working"),
  input: tr("needs you"),
  idle: tr("ready"),
  done: tr("done"),
  shell: tr("shell"),
};

export default function TabStrip({ session, tab, pane, onSelect, onNewTab, onNewTerminal, onCloseTab, onRename, tools, readOnly = false, masked = false }: {
  masked?: boolean; // screen-sharing mode over this workspace: no names of the person's own, no pane titles
  readOnly?: boolean; // a viewer's page: no new tab, no close, no rename
  session: Session;
  tab: Tab;
  pane: Pane;
  tools?: ReactNode;
  onSelect: (paneId: string) => void;
  onNewTab: () => void; // the dialog for an agent in a new tab
  onNewTerminal: () => void; // a shell tab in the workspace's folder, no questions
  onCloseTab: (t: Tab) => void;
  onRename: (t: Tab, label: string) => void;
}) {
  const [editing, setEditing] = useState<{ id: string; value: string } | null>(null);
  const faces = tabFaces(session.tabs, masked);
  const commit = () => {
    if (!editing) return;
    const t = session.tabs.find((x) => x.id === editing.id);
    const v = editing.value.trim();
    setEditing(null);
    if (t && v && v !== (t.named ? t.label : "")) onRename(t, v);
  };
  return (
    <div className="tabstrip">
      <div className="tabs-row">
        <div className="tabs" role="tablist">
          {session.tabs.map((t) => {
            const first = t.panes.find((p) => p.focused) ?? t.panes[0];
            if (editing?.id === t.id) {
              return (
                <div key={t.id} className="tab on editing">
                  <input
                    className="tab-edit"
                    autoFocus
                    value={editing.value}
                    placeholder={tr("Tab {n}", { n: t.number })}
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
            const face = faces.get(t.id)!;
            return (
              <div key={t.id} className={`tab${t.id === tab.id ? " on" : ""}${face.agent ? "" : " is-shell"}`}>
                <button
                  type="button"
                  role="tab"
                  aria-selected={t.id === tab.id}
                  className="tab-main"
                  disabled={!first}
                  title={readOnly ? `${tr("Tab {n}", { n: t.number })} · ${STATUS_LABEL[t.status]}` : `${tr("Tab {n}", { n: t.number })} · ${STATUS_LABEL[t.status]} — ${t.named ? tr("double-click to rename") : tr("double-click to name it")}`}
                  onClick={() => first && onSelect(first.id)}
                  onDoubleClick={readOnly ? undefined : () => setEditing({ id: t.id, value: t.named ? t.label : "" })}
                >
                  {face.agent ? <Logo agent={face.agent} /> : <Icon name="terminal" size={12} />}
                  <span className="tab-label">{face.label}</span>
                  {face.agent && <StatusDot status={t.status} title={STATUS_LABEL[t.status]} />}
                  {t.panes.length > 1 && <span className="tab-count">{t.panes.length}</span>}
                </button>
                {!readOnly && <Tip text={tr("Close this tab")}>
                  <button type="button" className="icon-btn tab-close" aria-label={tr("Close tab {tab}", { tab: face.label })} onClick={() => onCloseTab(t)}>
                    <Icon name="x" size={12} />
                  </button>
                </Tip>}
              </div>
            );
          })}
          {!readOnly && (
            <Menu
              label={tr("New tab")}
              align="start"
              tip={tr("New tab: a terminal in this folder, or an agent")}
              trigger={
                <button type="button" className="icon-btn tab-add" aria-label={tr("New tab")}>
                  <Icon name="plus" size={14} />
                </button>
              }
              items={[
                { label: tr("Terminal"), icon: "terminal", hint: tr("a shell in the workspace's folder"), onSelect: onNewTerminal },
                { label: tr("Agent…"), icon: "sparkles", hint: tr("an agent in a new tab, its model and first prompt"), onSelect: onNewTab },
              ]}
            />
          )}
        </div>
        {tools && <div className="tabs-tools">{tools}</div>}
      </div>
      {tab.panes.length > 1 && (
        <div className="split-panes">
          {tab.panes.map((p, i) => (
            <button key={p.id} type="button" className={`split-pane${p.id === pane.id ? " on" : ""}`} aria-pressed={p.id === pane.id} onClick={() => onSelect(p.id)} title={masked ? undefined : p.cwd}>
              <span className="tree-glyph">{i === tab.panes.length - 1 ? "└" : "├"}</span>
              <StatusDot status={p.status} title={STATUS_LABEL[p.status]} />
              <span>{masked ? (p.agent ? "" : tr("terminal")) : p.title}</span>
              {p.agent && (
                <span className="ws-agent" style={{ color: brandColor(p.agent) }}>
                  {VENDOR_NAME[p.agent] ?? p.agent}
                </span>
              )}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
