// The small pieces a task is drawn with: its chips, a project or status as a Select option, the priority's mark, a due-date field.

import { useEffect, useState, type CSSProperties } from "react";
import { Icon, Logo, Tip } from "@/ui";
import { PRIORITIES, type Field, type Label, type Project, type Status, type Task } from "./api";
import { FieldChip } from "./fields";
import { dueLabel, toLocalInput } from "./prompt";

export const projectOption = (p: Project) => ({
  value: p.id,
  label: p.title,
  icon: <span className="tasks-dot" style={{ background: p.hex_color ? `#${p.hex_color}` : "var(--line-strong)" }} />,
});

export const statusOption = (s: Status) => ({
  value: s.id,
  label: s.title,
  hint: s.kind === "done" ? "done" : undefined,
  icon: <span className="tasks-dot" style={{ background: `#${s.hex_color || "888"}` }} />,
});

export function labelStyle(l: Label): CSSProperties {
  const hex = l.hex_color || "888888";
  return { background: `#${hex}33`, borderColor: `#${hex}88` };
}

// PrioMark is the priority's mark: nothing, then exclamation marks that redden
export function PrioMark({ level }: { level: number }) {
  if (level === 0) return <span className="sel-glyph dim">—</span>;
  return <span className={`tasks-prio p${level}`}>{"!".repeat(Math.max(1, level - 1))}</span>;
}

// TaskChips is what a task shows beside its title, on a list row and a board card alike
export function TaskChips({ task: t, project, status, fields }: { task: Task; project?: Project; status?: Status; fields?: Field[] }) {
  const due = dueLabel(t);
  return (
    <>
      {(fields ?? []).filter((f) => f.on_card).map((f) => (
        <FieldChip key={f.id} field={f} value={t.fields?.[String(f.id)]} />
      ))}
      {t.source && (
        <Tip text={`Made by an agent on ${t.source}`}>
          <span className="badge quiet task-source" aria-label={`Made by an agent on ${t.source}`}>
            <Icon name="bot" size={12} />
          </span>
        </Tip>
      )}
      {t.run && t.run.state !== "gone" && t.run.state !== "failed" && (
        <span className={`tasks-row-run is-${t.run.state}`} title={`${t.run.agent} on ${t.run.branch}: ${t.run.state}`}>
          <Logo agent={t.run.agent} />
        </span>
      )}
      {t.priority >= 3 && (
        <span className={`tasks-prio p${t.priority}`} title={PRIORITIES[t.priority]}>
          {"!".repeat(t.priority - 2)}
        </span>
      )}
      {(t.labels ?? []).map((l) => (
        <span key={l.id} className="tasks-label" style={labelStyle(l)}>
          {l.title}
        </span>
      ))}
      {due && <span className={`tasks-due${due.over && !t.done ? " over" : ""}`}>{due.text}</span>}
      {status && (
        <span className="tasks-status" title="The column it is in">
          <span className="tasks-dot" style={{ background: `#${status.hex_color || "888"}` }} />
          {status.title}
        </span>
      )}
      {project && <span className="tasks-row-project">{project.title}</span>}
    </>
  );
}

// DueInput keeps the value while typed and sends it on blur or Enter, never while empty (the clear button unsets):
// a datetime-local field fires a change per segment typed and reads as "" while incomplete
export function DueInput({ value, onCommit }: { value: string; onCommit: (iso: string) => void }) {
  const [local, setLocal] = useState(toLocalInput(value));
  useEffect(() => setLocal(toLocalInput(value)), [value]);
  const commit = () => {
    if (!local) return setLocal(toLocalInput(value));
    if (local === toLocalInput(value)) return;
    onCommit(new Date(local).toISOString());
  };
  return (
    <input
      type="datetime-local"
      value={local}
      onChange={(e) => setLocal(e.target.value)}
      onBlur={commit}
      onKeyDown={(e) => {
        if (e.key === "Enter") (e.target as HTMLInputElement).blur();
        if (e.key === "Escape") setLocal(toLocalInput(value));
      }}
    />
  );
}

export function Loading() {
  return <div className="placeholder">Loading…</div>;
}
