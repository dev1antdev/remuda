// The board: a project's columns side by side, its tasks as cards — dragged on a desk, moved from a
// ⋯ menu on a phone, shown in the order given: the server's, or the one the last drop made.

import { useCallback, useMemo, useRef, useState, type DragEvent, type ReactNode } from "react";
import type { Project, Status, Task } from "@/features/tasks/api";
import Icon from "@/ui/icons";
import { Notice, Tip, useDismiss, useSubmit } from "@/ui";
import { longPress } from "./press";
import { pickHow, type Pick } from "./selection";
import { t as tr } from "@/lib/i18n";

export default function Board({ project, tasks, openId, draggable, picked, selecting, onOpen, onPick, onMove, onAdd, chips }: {
  project: Project;
  tasks: Task[]; // in column order
  openId: number | null;
  draggable: boolean; // a mouse can drag; a finger uses the menu
  picked?: (id: number) => boolean; // the cards selected for a bulk action
  selecting?: boolean; // a phone's selection is on: a tap picks instead of opening
  onOpen: (id: number) => void;
  onPick?: (task: Task, how: Pick) => void; // ⌘-click, ⇧-click, a long press
  // after: the card to sit behind in the column, 0 for the top, undefined for the bottom
  onMove: (task: Task, statusId: number, after: number | undefined) => void;
  onAdd: (title: string, statusId: number) => Promise<void>;
  chips: (t: Task) => ReactNode;
}) {
  const byStatus = useMemo(() => {
    const m = new Map<number, Task[]>();
    for (const t of tasks) {
      const list = m.get(t.status_id) ?? [];
      list.push(t);
      m.set(t.status_id, list);
    }
    return m;
  }, [tasks]);
  const [drag, setDrag] = useState<number | null>(null);
  const [over, setOver] = useState<{ status: number; index: number } | null>(null);
  const [adding, setAdding] = useState<number | null>(null);
  const clear = () => {
    setDrag(null);
    setOver(null);
  };

  // indexAt is the index a dropped card would take at y, the card being dragged not counted
  const indexAt = (col: HTMLElement, y: number) => {
    let i = 0;
    for (const c of col.querySelectorAll<HTMLElement>(".board-card")) {
      if (Number(c.dataset.id) === drag) continue;
      const r = c.getBoundingClientRect();
      if (y < r.top + r.height / 2) break;
      i++;
    }
    return i;
  };
  const drop = (statusId: number, index: number) => {
    const t = tasks.find((x) => x.id === drag);
    clear();
    if (!t) return;
    const column = byStatus.get(statusId) ?? [];
    const was = column.findIndex((x) => x.id === t.id);
    if (was >= 0 && (was === index || was === index - 1)) return; // the same place
    const others = column.filter((x) => x.id !== t.id);
    const after = index <= 0 ? 0 : others[Math.min(index, others.length) - 1]!.id;
    onMove(t, statusId, after);
  };

  return (
    <div className="board">
      {project.statuses.map((s) => {
        const list = byStatus.get(s.id) ?? [];
        const here = over?.status === s.id;
        return (
          <section
            key={s.id}
            className={`board-col kind-${s.kind}${here ? " over" : ""}`}
            onDragOver={(e) => {
              if (drag === null) return;
              e.preventDefault();
              e.dataTransfer.dropEffect = "move";
              const index = indexAt(e.currentTarget, e.clientY);
              setOver((o) => (o && o.status === s.id && o.index === index ? o : { status: s.id, index }));
            }}
            onDragLeave={(e) => {
              if (!e.currentTarget.contains(e.relatedTarget as Node | null)) setOver((o) => (o?.status === s.id ? null : o));
            }}
            onDrop={(e) => {
              e.preventDefault();
              drop(s.id, indexAt(e.currentTarget, e.clientY));
            }}
          >
            <header className="board-col-head">
              <span className="tasks-dot" style={{ background: `#${s.hex_color || "888"}` }} />
              <span className="board-col-title">{s.title}</span>
              {s.on_agent && (
                <Tip text="What agents propose lands here, for you to accept or decline">
                  <span className="board-col-mark" aria-label={tr("Agents' proposals land here")}>
                    <Icon name="bot" size={12} />
                  </span>
                </Tip>
              )}
              <span className="badge quiet">{list.length}</span>
              <button type="button" className="icon-btn sm" title={`Add a task to ${s.title}`} aria-label={`Add a task to ${s.title}`} onClick={() => setAdding(s.id)}>
                <Icon name="plus" size={14} />
              </button>
            </header>
            <div className="board-col-body">
              {adding === s.id && <ColumnAdd onAdd={(title) => onAdd(title, s.id)} onClose={() => setAdding(null)} />}
              {list.map((t, i) => (
                <Card
                  key={t.id}
                  task={t}
                  statuses={project.statuses}
                  selected={t.id === openId}
                  picked={!!picked?.(t.id)}
                  selecting={!!selecting}
                  dragging={t.id === drag}
                  before={here && over!.index === i}
                  draggable={draggable}
                  onOpen={() => onOpen(t.id)}
                  onPick={onPick && ((how) => onPick(t, how))}
                  onMoveTo={(statusId) => onMove(t, statusId, undefined)}
                  onDragStart={(e) => {
                    e.dataTransfer.setData("text/plain", String(t.id));
                    e.dataTransfer.effectAllowed = "move";
                    setDrag(t.id);
                  }}
                  onDragEnd={clear}
                  chips={chips}
                />
              ))}
              {here && over!.index >= list.length && <div className="board-drop-end" />}
            </div>
          </section>
        );
      })}
    </div>
  );
}

function Card({ task: t, statuses, selected, picked, selecting, dragging, before, draggable, onOpen, onPick, onMoveTo, onDragStart, onDragEnd, chips }: {
  task: Task;
  statuses: Status[];
  selected: boolean;
  picked: boolean;
  selecting: boolean;
  dragging: boolean;
  before: boolean; // a dropped card would land right above this one
  draggable: boolean;
  onOpen: () => void;
  onPick?: (how: Pick) => void;
  onMoveTo: (statusId: number) => void;
  onDragStart: (e: DragEvent<HTMLElement>) => void;
  onDragEnd: () => void;
  chips: (t: Task) => ReactNode;
}) {
  const [menu, setMenu] = useState(false);
  const closeMenu = useCallback(() => setMenu(false), []);
  useDismiss(menu, closeMenu, [".board-card-menu"]);
  const elsewhere = statuses.filter((s) => s.id !== t.status_id);
  const pickRef = useRef(onPick);
  pickRef.current = onPick;
  const press = useMemo(() => longPress(() => pickRef.current?.("toggle")), []);
  return (
    <article
      className={`board-card${selected ? " sel" : ""}${picked ? " picked" : ""}${dragging ? " dragging" : ""}${before ? " drop-before" : ""}${t.done ? " done" : ""}`}
      data-id={t.id}
      draggable={draggable}
      role="button"
      tabIndex={0}
      aria-expanded={selected}
      onMouseDown={(e) => {
        if (e.shiftKey && onPick) e.preventDefault(); // a range, not the browser's text selection
      }}
      onClick={(e) => {
        const how = onPick ? pickHow(e, selecting) : "open";
        if (how === "open") onOpen();
        else onPick!(how);
      }}
      {...press}
      onKeyDown={(e) => {
        if (e.target !== e.currentTarget) return;
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          onOpen();
        }
      }}
      onDragStart={onDragStart}
      onDragEnd={onDragEnd}
    >
      <div className="board-card-head">
        <span className="board-card-title">{t.title}</span>
        <span className="board-card-menu">
          <button
            type="button"
            className="icon-btn sm"
            title={tr("Move to…")}
            aria-label={tr("Move to another column")}
            aria-haspopup="menu"
            aria-expanded={menu}
            disabled={elsewhere.length === 0}
            onClick={(e) => {
              e.stopPropagation();
              setMenu((m) => !m);
            }}
          >
            <Icon name="more" size={14} />
          </button>
          {menu && (
            <span className="row-menu" role="menu">
              {elsewhere.map((s) => (
                <button
                  key={s.id}
                  type="button"
                  role="menuitem"
                  onClick={(e) => {
                    e.stopPropagation();
                    setMenu(false);
                    onMoveTo(s.id);
                  }}
                >
                  <span className="tasks-dot" style={{ background: `#${s.hex_color || "888"}` }} /> {s.title}
                </button>
              ))}
            </span>
          )}
        </span>
      </div>
      <div className="board-card-chips">
        <span className="tasks-ident">{t.identifier}</span>
        {chips(t)}
      </div>
    </article>
  );
}

// a task typed straight into a column
function ColumnAdd({ onAdd, onClose }: { onAdd: (title: string) => Promise<void>; onClose: () => void }) {
  const [value, setValue] = useState("");
  const { busy, err, run } = useSubmit(async () => {
    await onAdd(value.trim());
    setValue("");
  });
  return (
    <form
      className="board-add"
      onSubmit={(e) => {
        e.preventDefault();
        if (value.trim() && !busy) void run();
      }}
    >
      <input
        autoFocus
        value={value}
        placeholder={tr("A task…")}
        disabled={busy}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Escape") onClose();
        }}
        onBlur={() => {
          if (!value.trim()) onClose();
        }}
      />
      {err && <Notice tone="error">{err}</Notice>}
    </form>
  );
}
