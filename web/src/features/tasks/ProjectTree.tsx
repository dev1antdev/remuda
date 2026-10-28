// The projects as a tree in the mode's sidebar, favourites first, and the dialog that makes one.

import { useMemo, useState, type ReactNode } from "react";
import { plural } from "@/lib/text";
import { Dialog, Icon, Notice, Select, useSubmit } from "@/ui";
import { createProject, type Project } from "./api";
import { projectOption } from "./pieces";

export function ProjectTree({ projects, selected, onSelect }: { projects: Project[]; selected: number; onSelect: (id: number) => void }) {
  const children = useMemo(() => {
    const m = new Map<number, Project[]>();
    for (const p of projects) {
      if (p.is_archived) continue;
      const list = m.get(p.parent_project_id) ?? [];
      list.push(p);
      m.set(p.parent_project_id, list);
    }
    for (const list of m.values()) list.sort((a, b) => Number(b.is_favorite) - Number(a.is_favorite) || a.position - b.position);
    return m;
  }, [projects]);
  // roots: no parent, or a parent that is not shown (gone, or archived)
  const shown = new Set(projects.filter((p) => !p.is_archived).map((p) => p.id));
  const roots = [...(children.get(0) ?? []), ...[...children.entries()].filter(([pid]) => pid !== 0 && !shown.has(pid)).flatMap(([, l]) => l)];
  const render = (list: Project[], depth: number): ReactNode =>
    list.map((p) => (
      <div key={p.id}>
        <button
          type="button"
          className={`tasks-nav project${p.id === selected ? " sel" : ""}`}
          style={{ paddingLeft: 12 + depth * 14 }}
          onClick={() => onSelect(p.id)}
          title={p.description || p.title}
        >
          <span className="tasks-dot" style={{ background: p.hex_color ? `#${p.hex_color}` : "var(--line-strong)" }} />
          <span className="tasks-nav-label">{p.title}</span>
          {p.proposed > 0 && (
            <span className="badge quiet" title={`${plural(p.proposed, "task")} proposed by agents`}>
              {p.proposed}
            </span>
          )}
          {p.is_favorite && <Icon name="star" size={12} className="tasks-star" />}
        </button>
        {render(children.get(p.id) ?? [], depth + 1)}
      </div>
    ));
  return <div className="tasks-tree">{render(roots, 0)}</div>;
}

export function NewProjectDialog({ projects, onClose, onCreated }: { projects: Project[]; onClose: () => void; onCreated: (p: Project) => void }) {
  const [title, setTitle] = useState("");
  const [parent, setParent] = useState(0);
  const { busy, err, run } = useSubmit(async () => onCreated(await createProject(title.trim(), parent)));
  return (
    <Dialog
      title="New project"
      icon="folder"
      as="form"
      onClose={onClose}
      onSubmit={(e) => {
        e.preventDefault();
        if (title.trim()) void run();
      }}
    >
      <label>
        Name
        <input value={title} onChange={(e) => setTitle(e.target.value)} autoFocus required />
      </label>
      <label>
        Inside
        <Select value={parent} onChange={setParent} options={[{ value: 0, label: "— top level —" }, ...projects.filter((p) => !p.is_archived).map(projectOption)]} />
      </label>
      {err && <Notice tone="error">{err}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          Cancel
        </button>
        <button type="submit" className="btn primary" disabled={busy || !title.trim()}>
          {busy ? "Creating…" : "Create"}
        </button>
      </div>
    </Dialog>
  );
}
