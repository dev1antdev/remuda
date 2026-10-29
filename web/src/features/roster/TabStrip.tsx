// TabStrip is the workspace's tabs above the pane, as in herdr; a split tab lists its panes under the row.
// A lens's own controls go in the right slot (tools), never in the header.

import { useState, type ReactNode } from "react";
import { Icon, StatusDot, Tip } from "@/ui";
import { brandColor } from "@/ui/brand";
import type { Pane, Session, Status, Tab } from "./api";
import { t as tr } from "@/lib/i18n";

const STATUS_LABEL: Record<Status, string> = {
  run: "working",
  input: tr("needs you"),
  idle: "ready",
  done: "done",
  shell: "shell",
};

export default function TabStrip({ session, tab, pane, onSelect, onNewTab, onCloseTab, onRename, tools, readOnly = false }: {
  readOnly?: boolean; // a viewer's page: no new tab, no close, no rename
  session: Session;
  tab: Tab;
  pane: Pane;
  tools?: ReactNode;
  onSelect: (paneId: string) => void;
  onNewTab: () => void;
  onCloseTab: (t: Tab) => void;
  onRename: (t: Tab, label: string) => void;
}) {
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
      <div className="tabs-row">
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
            return (
              <div key={t.id} className={`tab${t.id === tab.id ? " sel" : ""}`}>
                <button
                  type="button"
                  role="tab"
                  aria-selected={t.id === tab.id}
                  className="tab-main"
                  disabled={!first}
                  title={t.named ? tr("{label} — double-click to rename", { label: t.label }) : tr("Double-click to name this tab")}
                  onClick={() => first && onSelect(first.id)}
                  onDoubleClick={() => setEditing({ id: t.id, value: t.named ? t.label : "" })}
                >
                  <StatusDot status={t.status} title={STATUS_LABEL[t.status]} />
                  <span className="tab-num">{t.number}</span>
                  {t.named && <span>{t.label}</span>}
                  {t.panes.length > 1 && <span className="tab-count">{t.panes.length}</span>}
                </button>
                {!readOnly && <Tip text={tr("Close this tab")}>
                  <button type="button" className="icon-btn tab-close" aria-label={tr("Close tab {tab}", { tab: t.named ? t.label : t.number })} onClick={() => onCloseTab(t)}>
                    <Icon name="x" size={12} />
                  </button>
                </Tip>}
              </div>
            );
          })}
          {!readOnly && <Tip text={tr("New tab in this workspace")}>
            <button type="button" className="icon-btn tab-add" aria-label={tr("New tab")} onClick={onNewTab}>
              <Icon name="plus" size={14} />
            </button>
          </Tip>}
        </div>
        {tools && <div className="tabs-tools">{tools}</div>}
      </div>
      {tab.panes.length > 1 && (
        <div className="split-panes">
          {tab.panes.map((p, i) => (
            <button key={p.id} type="button" className={`split-pane${p.id === pane.id ? " sel" : ""}`} aria-pressed={p.id === pane.id} onClick={() => onSelect(p.id)} title={p.cwd}>
              <span className="tree-glyph">{i === tab.panes.length - 1 ? "└" : "├"}</span>
              <StatusDot status={p.status} title={STATUS_LABEL[p.status]} />
              <span>{p.title}</span>
              {p.agent && (
                <span className="ws-agent" style={{ color: brandColor(p.agent) }}>
                  {p.agent}
                </span>
              )}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
