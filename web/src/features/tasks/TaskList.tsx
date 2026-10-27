// The list: a quick-add line above it, one row per task with its checkbox and chips.

import { useState, type ReactNode } from "react";
import { Icon, Notice, useSubmit } from "@/ui";
import type { Field, Project, Status, Task } from "./api";
import { TaskChips } from "./pieces";
import type { RunPR } from "./runPR";

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

export function TaskRow({ task: t, project, status, fields, pr, selected, onToggle, onOpen, actions }: {
  task: Task;
  project?: Project;
  status?: Status;
  fields?: Field[];
  pr?: RunPR | null; // the pull request the task's run noted
  selected: boolean;
  onToggle: () => void;
  onOpen: () => void;
  actions?: ReactNode; // the row's own controls, at its end (the triage of a proposal)
}) {
  return (
    <div
      className={`tasks-row${t.done ? " done" : ""}${selected ? " sel" : ""}${actions ? " with-actions" : ""}`}
      role="button"
      tabIndex={0}
      aria-expanded={selected}
      onClick={onOpen}
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
      <TaskChips task={t} project={project} status={status} fields={fields} pr={pr} />
      {actions}
    </div>
  );
}
