// A project's settings: its name and colour, and its workflow — the columns a task moves through,
// each open or done, and the moves the workflow makes by itself — plus its own fields. Saved whole;
// a removed column says where its tasks go.

import { useCallback, useState } from "react";
import { FIELD_KINDS, patchProject, putFields, putStatuses, type Field, type FieldKind, type Project, type Status } from "./tasksApi";
import Icon, { type IconName } from "./icons";
import Select from "./Select";
import { Dialog, Notice, useDismiss, useSubmit } from "./ui";

export const PALETTE = ["e67e22", "2ecc71", "3498db", "9b59b6", "e74c3c", "1abc9c", "f1c40f", "95a5a6"];
export const randomColor = () => PALETTE[Math.floor(Math.random() * PALETTE.length)]!;

type Flag = "on_new" | "on_start" | "on_finish";
const FLAGS: { flag: Flag; icon: IconName; title: string }[] = [
  { flag: "on_new", icon: "inbox", title: "New tasks land here" },
  { flag: "on_start", icon: "play", title: "A task an agent is started on moves here" },
  { flag: "on_finish", icon: "circle-check", title: "A task whose agent finished moves here" },
];

// Row is a column as edited: its key outlives a rename, a removed one keeps its row so the choice of where its tasks go sits beside it
type Row = Status & { key: number; removed?: boolean; to?: number };
// a field as edited: the options as the text typed, split on save
type FRow = Field & { key: number; optionsText: string };

// the fields' rows go up and down, as the columns' do
function shifted<T extends { key: number }>(rs: T[], key: number, dir: -1 | 1): T[] {
  const i = rs.findIndex((r) => r.key === key);
  const j = i + dir;
  if (i < 0 || j < 0 || j >= rs.length) return rs;
  const next = [...rs];
  [next[i], next[j]] = [next[j]!, next[i]!];
  return next;
}

export default function ProjectSettings({ project, onClose, onSaved }: { project: Project; onClose: () => void; onSaved: (p: Project) => void }) {
  const [title, setTitle] = useState(project.title);
  const [color, setColor] = useState(project.hex_color);
  const [rows, setRows] = useState<Row[]>(() => project.statuses.map((s, i) => ({ ...s, key: i + 1 })));
  const [nextKey, setNextKey] = useState(project.statuses.length + 1);

  const update = (key: number, patch: Partial<Row>) => setRows((rs) => rs.map((r) => (r.key === key ? { ...r, ...patch } : r)));
  // one column per flag: setting it here clears it elsewhere
  const flag = (key: number, which: Flag, on: boolean) => setRows((rs) => rs.map((r) => ({ ...r, [which]: r.key === key ? on : on ? false : r[which] })));
  const shift = (key: number, dir: -1 | 1) => setRows((rs) => shifted(rs, key, dir));
  const remove = (key: number) =>
    setRows((rs) => {
      const r = rs.find((x) => x.key === key);
      if (!r) return rs;
      return r.id === 0 ? rs.filter((x) => x.key !== key) : rs.map((x) => (x.key === key ? { ...x, removed: true } : x));
    });
  const add = () => {
    setRows((rs) => [...rs, { id: 0, project_id: project.id, title: "", hex_color: randomColor(), kind: "open", position: 0, on_new: false, on_start: false, on_finish: false, key: nextKey }]);
    setNextKey((k) => k + 1);
  };

  const [frows, setFrows] = useState<FRow[]>(() => project.fields.map((f, i) => ({ ...f, key: i + 1, optionsText: f.options.join(", ") })));
  const [nextFKey, setNextFKey] = useState(project.fields.length + 1);
  const fupdate = (key: number, patch: Partial<FRow>) => setFrows((rs) => rs.map((r) => (r.key === key ? { ...r, ...patch } : r)));
  const fadd = () => {
    setFrows((rs) => [...rs, { id: 0, project_id: project.id, title: "", kind: "text", options: [], position: 0, on_card: false, in_prompt: true, key: nextFKey, optionsText: "" }]);
    setNextFKey((k) => k + 1);
  };
  const fieldsChanged = () =>
    frows.length !== project.fields.length ||
    frows.some((r, i) => {
      const was = project.fields[i];
      return !was || was.id !== r.id || was.title !== r.title || was.kind !== r.kind || was.on_card !== r.on_card || was.in_prompt !== r.in_prompt || was.options.join(", ") !== r.optionsText;
    });

  const kept = rows.filter((r) => !r.removed);
  const survivors = kept.filter((r) => r.id !== 0); // where a removed column's tasks can go
  const ok =
    !!title.trim() &&
    kept.length > 0 &&
    kept.every((r) => r.title.trim()) &&
    kept.some((r) => r.kind === "done") &&
    kept.some((r) => r.kind !== "done") &&
    frows.every((r) => r.title.trim());
  const { busy, err, run } = useSubmit(async () => {
    if (title.trim() !== project.title || color !== project.hex_color) await patchProject(project.id, { title: title.trim(), hex_color: color });
    const moves: Record<number, number> = {};
    for (const r of rows) if (r.removed) moves[r.id] = r.to ?? survivors[0]?.id ?? 0;
    let saved = await putStatuses(
      project.id,
      kept.map(({ key: _key, removed: _removed, to: _to, ...s }) => s),
      moves,
    );
    if (fieldsChanged()) {
      saved = await putFields(
        project.id,
        frows.map(({ key: _key, optionsText, ...f }) => ({
          ...f,
          options:
            f.kind === "select" || f.kind === "multi"
              ? optionsText
                  .split(",")
                  .map((s) => s.trim())
                  .filter(Boolean)
              : [],
        })),
      );
    }
    onSaved(saved);
  });

  return (
    <Dialog
      title="Project settings"
      icon="settings"
      size="lg"
      as="form"
      className="project-settings"
      onClose={onClose}
      onSubmit={(e) => {
        e.preventDefault();
        if (ok && !busy) void run();
      }}
    >
      <div className="schedule-row">
        <label className="grow">
          Name
          <input value={title} onChange={(e) => setTitle(e.target.value)} required />
        </label>
        <label>
          Colour
          <ColorPick value={color} onChange={setColor} />
        </label>
      </div>

      <div className="tasks-field-name">Columns</div>
      <p className="modal-hint">
        A task is always in one column, and a column that is <b>done</b> makes its tasks done. The three marks on a column say what the workflow does by itself: where new tasks
        land, where a task goes when an agent is started on it, where it goes when that agent finishes — unless you moved it meanwhile.
      </p>
      <div className="ps-rows">
        {rows.map((r, i) => (
          <div key={r.key} className={`ps-row${r.removed ? " removed" : ""}`}>
            <ColorPick value={r.hex_color} onChange={(c) => update(r.key, { hex_color: c })} />
            <input className="ps-name" value={r.title} placeholder="Column name" disabled={r.removed} onChange={(e) => update(r.key, { title: e.target.value })} />
            <Select<"open" | "done">
              className="sm"
              value={r.kind}
              onChange={(kind) => update(r.key, { kind })}
              options={[
                { value: "open", label: "Open" },
                { value: "done", label: "Done" },
              ]}
            />
            <span className="ps-flags">
              {FLAGS.map((f) => (
                <button
                  key={f.flag}
                  type="button"
                  className={`icon-btn sm${r[f.flag] ? " on" : ""}`}
                  title={f.title}
                  aria-label={f.title}
                  aria-pressed={r[f.flag]}
                  disabled={r.removed}
                  onClick={() => flag(r.key, f.flag, !r[f.flag])}
                >
                  <Icon name={f.icon} size={14} />
                </button>
              ))}
            </span>
            <button type="button" className="icon-btn sm" title="Move up" aria-label="Move up" disabled={i === 0} onClick={() => shift(r.key, -1)}>
              <Icon name="chevron-up" size={14} />
            </button>
            <button type="button" className="icon-btn sm" title="Move down" aria-label="Move down" disabled={i === rows.length - 1} onClick={() => shift(r.key, 1)}>
              <Icon name="chevron-down" size={14} />
            </button>
            {r.removed ? (
              <span className="ps-moves">
                <Select
                  className="sm"
                  value={r.to ?? survivors[0]?.id ?? 0}
                  onChange={(to) => update(r.key, { to })}
                  options={survivors.map((s) => ({ value: s.id, label: `its tasks → ${s.title}` }))}
                />
                <button type="button" className="linkish" onClick={() => update(r.key, { removed: false })}>
                  undo
                </button>
              </span>
            ) : (
              <button type="button" className="icon-btn sm" title="Remove column" aria-label="Remove column" disabled={kept.length <= 1} onClick={() => remove(r.key)}>
                <Icon name="x" size={14} />
              </button>
            )}
          </div>
        ))}
      </div>
      <div>
        <button type="button" className="btn sm" onClick={add}>
          <Icon name="plus" size={12} /> Column
        </button>
      </div>
      {!ok && kept.length > 0 && (!kept.some((r) => r.kind === "done") || !kept.some((r) => r.kind !== "done")) && (
        <Notice tone="hint">A project needs an open column and a done column.</Notice>
      )}

      <div className="tasks-field-name">Fields</div>
      <p className="modal-hint">
        What this project's tasks carry besides a title, description, due date, priority and labels. A field <b>on the card</b> shows beside the title; one <b>in the prompt</b>{" "}
        goes to the agent with the task. A removed field takes its values with it, and so does a change of type.
      </p>
      <div className="ps-rows">
        {frows.map((r, i) => (
          <div key={r.key} className="ps-row">
            <input className="ps-name" value={r.title} placeholder="Field name" onChange={(e) => fupdate(r.key, { title: e.target.value })} />
            <Select<FieldKind> className="sm" value={r.kind} onChange={(kind) => fupdate(r.key, { kind })} options={FIELD_KINDS} />
            {(r.kind === "select" || r.kind === "multi") && (
              <input className="ps-options" value={r.optionsText} placeholder="options, comma-separated" onChange={(e) => fupdate(r.key, { optionsText: e.target.value })} />
            )}
            <span className="ps-flags">
              <button
                type="button"
                className={`icon-btn sm${r.on_card ? " on" : ""}`}
                title="Show on the card and the list row"
                aria-label="Show on the card"
                aria-pressed={r.on_card}
                onClick={() => fupdate(r.key, { on_card: !r.on_card })}
              >
                <Icon name="eye" size={14} />
              </button>
              <button
                type="button"
                className={`icon-btn sm${r.in_prompt ? " on" : ""}`}
                title="Send to the agent with the task"
                aria-label="Send to the agent"
                aria-pressed={r.in_prompt}
                onClick={() => fupdate(r.key, { in_prompt: !r.in_prompt })}
              >
                <Icon name="send" size={14} />
              </button>
            </span>
            <button type="button" className="icon-btn sm" title="Move up" aria-label="Move up" disabled={i === 0} onClick={() => setFrows((rs) => shifted(rs, r.key, -1))}>
              <Icon name="chevron-up" size={14} />
            </button>
            <button type="button" className="icon-btn sm" title="Move down" aria-label="Move down" disabled={i === frows.length - 1} onClick={() => setFrows((rs) => shifted(rs, r.key, 1))}>
              <Icon name="chevron-down" size={14} />
            </button>
            <button type="button" className="icon-btn sm" title="Remove field" aria-label="Remove field" onClick={() => setFrows((rs) => rs.filter((x) => x.key !== r.key))}>
              <Icon name="x" size={14} />
            </button>
          </div>
        ))}
      </div>
      <div>
        <button type="button" className="btn sm" onClick={fadd}>
          <Icon name="plus" size={12} /> Field
        </button>
      </div>
      {err && <Notice tone="error">{err}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          Cancel
        </button>
        <button type="submit" className="btn primary" disabled={busy || !ok}>
          {busy ? "Saving…" : "Save"}
        </button>
      </div>
    </Dialog>
  );
}

// a colour from the palette, behind a dot
export function ColorPick({ value, onChange }: { value: string; onChange: (hex: string) => void }) {
  const [open, setOpen] = useState(false);
  const close = useCallback(() => setOpen(false), []);
  useDismiss(open, close, [".color-pick"]);
  return (
    <span className="color-pick">
      <button type="button" className="icon-btn sm" title="Colour" aria-label="Colour" aria-haspopup="menu" aria-expanded={open} onClick={() => setOpen((o) => !o)}>
        <span className="tasks-dot lg" style={{ background: value ? `#${value}` : "var(--line-strong)" }} />
      </button>
      {open && (
        <span className="row-menu color-menu" role="menu">
          {PALETTE.map((c) => (
            <button
              key={c}
              type="button"
              role="menuitem"
              className={c === value ? "on" : ""}
              aria-label={`#${c}`}
              onClick={() => {
                onChange(c);
                setOpen(false);
              }}
            >
              <span className="tasks-dot lg" style={{ background: `#${c}` }} />
            </button>
          ))}
        </span>
      )}
    </span>
  );
}
