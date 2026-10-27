// The list: a quick-add line above it, one row per task with its checkbox and chips.

import { useMemo, useRef, useState, type ReactNode } from "react";
import { Icon, Notice, useSubmit } from "@/ui";
import type { Field, Project, Status, Task } from "./api";
import { TaskChips } from "./pieces";
import { longPress } from "./press";
import { pickHow, type Pick } from "./selection";

export function QuickAdd({ placeholder, disabled, onAdd }: { placeholder: string; disabled: boolean; onAdd: (title: string) => Promise<void> }) {
  const [value, setValue] = useState("");
  const { busy, err, run, setErr } = useSubmit(async () => {
    await onAdd(value.trim());
    setValue("");
  });
  return (
    <>
      <form
        className="tasks-add"
        onSubmit={(e) => {
          e.preventDefault();
          if (value.trim() && !busy) void run();
        }}
      >
        <span className="tasks-add-plus">
          <Icon name="plus" size={14} />
        </span>
        <input value={value} onChange={(e) => setValue(e.target.value)} placeholder={placeholder} disabled={disabled || busy} />
      </form>
      {err && (
        <Notice tone="error" className="tasks-notice" onDismiss={() => setErr("")}>
          {err}
        </Notice>
      )}
    </>
  );
}

export function TaskRow({ task: t, project, status, fields, selected, picked, selecting, onToggle, onOpen, onPick, actions }: {
  task: Task;
  project?: Project;
  status?: Status;
  fields?: Field[];
  selected: boolean; // open on the right
  picked?: boolean; // one of several selected for a bulk action
  selecting?: boolean; // a phone's selection is on: a tap picks instead of opening
  onToggle: () => void;
  onOpen: () => void;
  onPick?: (how: Pick) => void; // ⌘-click, ⇧-click, a long press
  actions?: ReactNode; // the row's own controls, at its end (the triage of a proposal)
}) {
  const pickRef = useRef(onPick);
  pickRef.current = onPick;
  const press = useMemo(() => longPress(() => pickRef.current?.("toggle")), []);
  return (
    <div
      className={`tasks-row${t.done ? " done" : ""}${selected ? " sel" : ""}${picked ? " picked" : ""}${actions ? " with-actions" : ""}`}
      role="button"
      tabIndex={0}
      aria-expanded={selected}
      onMouseDown={(e) => {
        if (e.shiftKey && onPick) e.preventDefault(); // a range, not the browser's text selection
      }}
      onClick={(e) => {
        const how = onPick ? pickHow(e, !!selecting) : "open";
        if (how === "open") onOpen();
        else onPick!(how);
      }}
      {...press}
      onKeyDown={(e) => {
        if (e.target !== e.currentTarget) return; // the checkbox handles its own keys
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onOpen();
        }
      }}
    >
      <input
        type="checkbox"
        className="tasks-check"
        checked={t.done}
        onChange={onToggle}
        onClick={(e) => e.stopPropagation()}
        title={t.done ? "Mark undone" : "Mark done"}
        aria-label={t.done ? "Mark undone" : "Mark done"}
      />
      <span className="tasks-row-title">{t.title}</span>
      <TaskChips task={t} project={project} status={status} fields={fields} />
      {actions}
    </div>
  );
}
