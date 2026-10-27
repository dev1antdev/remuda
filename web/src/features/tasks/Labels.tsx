// A task's labels: the ones on it, and a picker that finds or makes one.

import { useState } from "react";
import { errMsg } from "@/lib/text";
import { Icon } from "@/ui";
import { addTaskLabel, createLabel, removeTaskLabel, type Label, type Task } from "./api";
import { labelStyle } from "./pieces";
import { randomColor } from "./ProjectSettings";
import { t as tr } from "@/lib/i18n";

export function LabelsField({ task: t, labels, onChange, onLabelsChanged, onError }: {
  task: Task;
  labels: Label[]; // every label there is
  onChange: (t: Task) => void;
  onLabelsChanged: () => void; // a new label was made
  onError: (msg: string) => void;
}) {
  const [picking, setPicking] = useState(false);
  const [query, setQuery] = useState("");
  const addLabel = async (l: Label) => {
    setPicking(false);
    setQuery("");
    try {
      await addTaskLabel(t.id, l.id);
      onChange({ ...t, labels: [...(t.labels ?? []), l] });
    } catch (e) {
      onError(errMsg(e));
    }
  };
  const newLabel = async (name: string) => {
    name = name.trim();
    if (!name) return;
    try {
      const l = await createLabel(name, randomColor());
      onLabelsChanged();
      await addLabel(l);
    } catch (e) {
      onError(errMsg(e));
    }
  };
  const dropLabel = async (l: Label) => {
    try {
      await removeTaskLabel(t.id, l.id);
      onChange({ ...t, labels: (t.labels ?? []).filter((x) => x.id !== l.id) });
    } catch (e) {
      onError(errMsg(e));
    }
  };
  const have = new Set((t.labels ?? []).map((l) => l.id));
  const wanted = query.trim().toLowerCase();
  const candidates = labels.filter((l) => !have.has(l.id) && l.title.toLowerCase().includes(wanted));
  const exact = candidates.find((l) => l.title.toLowerCase() === wanted);
  const attached = (t.labels ?? []).some((l) => l.title.toLowerCase() === wanted);
  return (
    <div className="tasks-field labels">
      <span className="tasks-field-name">{tr("Labels")}</span>
      <span className="tasks-labels">
        {(t.labels ?? []).map((l) => (
          <span key={l.id} className="tasks-label" style={labelStyle(l)}>
            {l.title}
            <button type="button" className="icon-btn sm tasks-label-x" onClick={() => void dropLabel(l)} title={tr("Remove")} aria-label={`Remove ${l.title}`}>
              <Icon name="x" size={12} />
            </button>
          </span>
        ))}
        {picking ? (
          <span className="tasks-label-pick">
            <input
              autoFocus
              value={query}
              placeholder={tr("label…")}
              onChange={(e) => setQuery(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Escape") setPicking(false);
                if (e.key === "Enter") {
                  e.preventDefault();
                  const q = query.trim();
                  if (exact) void addLabel(exact);
                  else if (!q && candidates[0]) void addLabel(candidates[0]);
                  else if (q && !attached) void newLabel(q);
                }
              }}
              onBlur={() => setTimeout(() => setPicking(false), 150)}
            />
            <span className="row-menu label-menu" role="listbox">
              {candidates.slice(0, 8).map((l) => (
                <button key={l.id} type="button" onMouseDown={(e) => e.preventDefault()} onClick={() => void addLabel(l)}>
                  <span className="tasks-dot" style={{ background: `#${l.hex_color || "888"}` }} /> {l.title}
                </button>
              ))}
              {query.trim() && !exact && !attached && (
                <button type="button" onMouseDown={(e) => e.preventDefault()} onClick={() => void newLabel(query)}>
                  <Icon name="plus" size={12} /> create “{query.trim()}”
                </button>
              )}
            </span>
          </span>
        ) : (
          <button type="button" className="linkish" onClick={() => setPicking(true)}>
            <Icon name="plus" size={12} /> label
          </button>
        )}
      </span>
    </div>
  );
}
