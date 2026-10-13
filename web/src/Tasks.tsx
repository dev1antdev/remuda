// The Tasks mode: remuda's own task list (internal/tasks). Projects on the left with three smart lists
// above them, the list or board in the middle, the open task on the right. Nothing is cached beyond the
// screen: every change goes to the server and the list is re-read.

import { createElement, useCallback, useEffect, useMemo, useState, type CSSProperties, type ReactNode } from "react";
import {
  addComment,
  addTaskLabel,
  createLabel,
  createProject,
  createTask,
  deleteComment,
  deleteTask,
  getTask,
  listLabels,
  listProjects,
  listTasks,
  patchTask,
  PRIORITIES,
  removeTaskLabel,
  startTask,
  type Field,
  type Label,
  type Project,
  type Run,
  type Status,
  type Task,
} from "./tasksApi";
import { FieldChip, FieldInput, valueText } from "./fields";
import type { Machine } from "./api";
import { errMsg, isTouch, useDebounced, usePref } from "./prefs";
import { openUrl } from "./host";
import { Markdown } from "./markdown";
import Select from "./Select";
import SendToAgent from "./SendToAgent";
import Board from "./Board";
import ProjectSettings, { randomColor } from "./ProjectSettings";
import Logo from "./Logo";
import Icon, { type IconName } from "./icons";
import { Dialog, Notice, useDismiss, useSubmit, useTimedNotice } from "./ui";
import { machineLabel, machineOptions } from "./options";
import { fmtDate, whenShort } from "./time";

type Smart = "today" | "upcoming" | "open";
type Sel = { kind: "smart"; id: Smart } | { kind: "project"; id: number };

// the smart lists: due by the end of today, due within a week, everything open
const SMART: { id: Smart; label: string; icon: IconName }[] = [
  { id: "today", label: "Today", icon: "sun" },
  { id: "upcoming", label: "Upcoming", icon: "calendar" },
  { id: "open", label: "All open", icon: "circle-check" },
];

const TZ = Intl.DateTimeFormat().resolvedOptions().timeZone;

// an agent pane from herdr's roster, flattened for the picker
export type AgentTarget = {
  machineId: string;
  machineLabel: string;
  sessionName: string;
  paneTitle: string; // the tab's name, or the agent's own title for its conversation
  paneId: string;
  agent: string;
  status: string;
  cwd: string;
};

export default function Tasks({ drawer, closeDrawer, ask, machines, agents, currentPane, onOpenPane, onSend }: {
  drawer: boolean;
  closeDrawer: () => void;
  ask: (text: string, label: string, run: () => void) => void;
  machines: Machine[];
  agents: AgentTarget[];
  currentPane: { machineId: string; paneId: string } | null;
  onOpenPane: (machineId: string, paneId: string) => void;
  onSend: (machineId: string, paneId: string, text: string) => Promise<void>;
}) {
  const [projects, setProjects] = useState<Project[] | null>(null);
  const [labels, setLabels] = useState<Label[]>([]);
  const [defaultProject, setDefaultProject] = useState(0);
  const [sel, setSel] = usePref<Sel>("tasks.sel", { kind: "smart", id: "today" });
  const [showDone, setShowDone] = usePref("tasks.showDone", false);
  const [view, setView] = usePref<"list" | "board">("tasks.view", "board");
  const [settings, setSettings] = useState(false);
  const [search, setSearch] = useState("");
  const [tasks, setTasks] = useState<Task[]>([]);
  const [pages, setPages] = useState(1);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [openId, setOpenId] = useState<number | null>(null);
  const [newProject, setNewProject] = useState(false);
  const [sendReq, setSendReq] = useState<{ task: Task; groom: boolean } | null>(null);
  const [startReq, setStartReq] = useState<Task | null>(null);
  const [notice, showNotice, clearNotice] = useTimedNotice(12000);
  const [noticeAt, setNoticeAt] = useState<{ machineId: string; paneId: string } | null>(null);
  const announce = (text: string, at: { machineId: string; paneId: string }) => {
    setNoticeAt(at);
    showNotice(text);
  };
  const [tick, setTick] = useState(0);
  const refresh = useCallback(() => setTick((t) => t + 1), []);
  // the list alone, re-read: a card moved, a column changed
  const [listTick, setListTick] = useState(0);
  const reloadList = useCallback(() => setListTick((t) => t + 1), []);

  useEffect(() => {
    let gone = false;
    void Promise.all([listProjects(), listLabels()]).then(
      ([ps, ls]) => {
        if (gone) return;
        setProjects(ps);
        setLabels(ls);
        setDefaultProject(ps.find((p) => p.title === "Inbox" && !p.is_archived)?.id || ps.find((p) => !p.is_archived)?.id || 0);
      },
      (e) => !gone && setError(errMsg(e)),
    );
    return () => {
      gone = true;
    };
  }, [tick]);

  const board = sel.kind === "project" && view === "board";
  const withDone = showDone && sel.kind === "project" && !board;
  const query = useMemo(
    () => (sel.kind === "smart" ? { view: sel.id } : board ? { view: "board" as const, project: sel.id } : { view: "project" as const, project: sel.id, done: withDone }),
    [sel, withDone, board],
  );
  const debounced = useDebounced(search, 250);
  const select = (next: Sel) => {
    setSel(next);
    setPage(1);
    setOpenId(null);
    closeDrawer();
  };
  useEffect(() => {
    let gone = false;
    setLoading(true);
    void listTasks({ ...query, search: debounced, page, timezone: TZ }).then(
      (r) => {
        if (gone) return;
        // the list knows no comments; an open task keeps the ones it loaded
        setTasks((prev) => r.tasks.map((t) => ({ ...t, comments: t.comments ?? prev.find((p) => p.id === t.id)?.comments })));
        setPages(r.pages);
        if (page > r.pages) setPage(Math.max(1, r.pages));
        setError("");
        setLoading(false);
      },
      (e) => {
        if (gone) return;
        setError(errMsg(e));
        setLoading(false);
      },
    );
    return () => {
      gone = true;
    };
  }, [query, debounced, page, tick, listTick]);
  useEffect(() => {
    if (!board) return;
    const timer = setInterval(() => {
      if (document.visibilityState === "visible") reloadList();
    }, 15000);
    return () => clearInterval(timer);
  }, [board, reloadList]);

  const byId = useMemo(() => new Map((projects ?? []).map((p) => [p.id, p])), [projects]);
  const project = sel.kind === "project" ? byId.get(sel.id) : undefined;
  const statusOf = (t: Task) => byId.get(t.project_id)?.statuses.find((s) => s.id === t.status_id);
  const open = openId === null ? null : (tasks.find((t) => t.id === openId) ?? null);

  const replace = (t: Task) => setTasks((ts) => ts.map((x) => (x.id === t.id ? t : x)));
  // replaceOrMove re-reads the list when the task left it — another project, another due date — or changed column, where the server knows the place
  const replaceOrMove = (t: Task) => {
    const was = tasks.find((x) => x.id === t.id);
    replace(t);
    if (was && (was.project_id !== t.project_id || was.due_date !== t.due_date)) refresh();
    else if (was && was.status_id !== t.status_id) reloadList();
  };

  // the open task in full (its comments), re-read while an agent is on it
  const live = !!open?.run && open.run.state !== "gone" && open.run.state !== "failed";
  useEffect(() => {
    if (openId === null) return;
    let gone = false;
    const load = () => void getTask(openId).then((t) => !gone && replace(t), () => {});
    load();
    const timer = live ? setInterval(load, 10000) : null;
    return () => {
      gone = true;
      if (timer) clearInterval(timer);
    };
  }, [openId, live, tick]);

  const toggleDone = async (t: Task) => {
    const want = !t.done;
    replace({ ...t, done: want });
    try {
      replace(await patchTask(t.id, { done: want }));
    } catch (e) {
      replace(t);
      setError(errMsg(e));
    }
  };

  const addTask = async (title: string) => {
    const projectId = sel.kind === "project" && sel.id > 0 ? sel.id : defaultProject;
    if (!projectId) throw new Error("no project to add to");
    const fields: Partial<Task> & { title: string } = { title };
    if (sel.kind === "smart" && sel.id === "today") fields.due_date = endOfToday();
    await createTask(projectId, fields);
    refresh();
  };
  const addTo = async (title: string, statusId: number) => {
    if (sel.kind !== "project") return;
    await createTask(sel.id, { title, status_id: statusId });
    reloadList();
  };
  // a dropped card is shown there at once; the server is told and the list put back as it was if it refuses
  const moveCard = (t: Task, statusId: number, after: number | undefined) => {
    const before = tasks;
    setTasks((ts) => {
      const rest = ts.filter((x) => x.id !== t.id);
      const moved = { ...t, status_id: statusId };
      let at: number;
      if (after === undefined) {
        at = rest.reduce((last, x, i) => (x.status_id === statusId ? i + 1 : last), -1); // after the column's last card
        if (at < 0) at = rest.length;
      } else if (after === 0) {
        at = rest.findIndex((x) => x.status_id === statusId);
        if (at < 0) at = rest.length;
      } else {
        at = rest.findIndex((x) => x.id === after) + 1;
      }
      rest.splice(at, 0, moved);
      return rest;
    });
    void patchTask(t.id, { status_id: statusId, after }).then(replace, (e) => {
      setTasks(before);
      setError(errMsg(e));
    });
  };

  const remove = (t: Task) =>
    ask(`Delete “${t.title}”?`, "Delete", () => {
      void deleteTask(t.id).then(() => {
        setOpenId(null);
        refresh();
      }, (e) => setError(errMsg(e)));
    });

  if (projects === null) {
    return (
      <main className="pane-area">
        {error ? (
          <Notice tone="error" icon="alert-triangle" className="tasks-notice">
            {error}
          </Notice>
        ) : (
          <div className="placeholder">Loading…</div>
        )}
      </main>
    );
  }

  const addTarget = sel.kind === "project" ? byId.get(sel.id) : byId.get(defaultProject);
  const title = sel.kind === "smart" ? SMART.find((s) => s.id === sel.id)!.label : (byId.get(sel.id)?.title ?? "…");

  return (
    <>
      <aside className={drawer ? "sidebar open" : "sidebar"}>
        <div className="sidebar-head">
          <span>Lists</span>
          <button type="button" className="btn sm" onClick={() => setNewProject(true)} title="New project">
            <Icon name="plus" size={12} /> Project
          </button>
        </div>
        {SMART.map((s) => (
          <button key={s.id} type="button" className={`tasks-nav${sel.kind === "smart" && sel.id === s.id ? " sel" : ""}`} onClick={() => select({ kind: "smart", id: s.id })}>
            <span className="tasks-nav-glyph">
              <Icon name={s.icon} size={14} />
            </span>
            {s.label}
          </button>
        ))}
        <div className="sidebar-head">
          <span>Projects</span>
        </div>
        <ProjectTree projects={projects} selected={sel.kind === "project" ? sel.id : 0} onSelect={(id) => select({ kind: "project", id })} />
      </aside>
      {drawer && <div className="scrim" onClick={closeDrawer} />}

      <main className="pane-area tasks-main">
        <div className="tasks-head">
          <h1>{title}</h1>
          {sel.kind === "project" && (
            <nav className="tasks-view" aria-label="View">
              <button type="button" className={view === "list" ? "on" : ""} onClick={() => setView("list")} title="As a list">
                <Icon name="list" size={14} /> List
              </button>
              <button type="button" className={view === "board" ? "on" : ""} onClick={() => setView("board")} title="As a board: a column per status">
                <Icon name="layers" size={14} /> Board
              </button>
            </nav>
          )}
          {sel.kind === "project" && !board && (
            <label className="tasks-toggle">
              <input
                type="checkbox"
                checked={showDone}
                onChange={(e) => {
                  setShowDone(e.target.checked);
                  setPage(1);
                }}
              />{" "}
              done
            </label>
          )}
          <input
            className="tasks-search"
            type="search"
            placeholder="Search…"
            value={search}
            onChange={(e) => {
              setSearch(e.target.value);
              setPage(1);
            }}
          />
          {project && (
            <button type="button" className="icon-btn" title="Project settings: name, colour, columns" aria-label="Project settings" onClick={() => setSettings(true)}>
              <Icon name="settings" size={16} />
            </button>
          )}
        </div>
        <QuickAdd placeholder={addTarget ? `Add a task to ${addTarget.title}…` : "Add a task…"} disabled={!addTarget} onAdd={addTask} />
        {error && (
          <Notice tone="error" icon="alert-triangle" className="tasks-notice" onDismiss={() => setError("")}>
            {error}
          </Notice>
        )}
        {notice && (
          <Notice
            tone="success"
            icon="circle-check"
            className="tasks-notice"
            onDismiss={clearNotice}
            action={
              noticeAt && (
                <button type="button" className="linkish" onClick={() => onOpenPane(noticeAt.machineId, noticeAt.paneId)}>
                  Open <Icon name="chevron-right" size={12} />
                </button>
              )
            }
          >
            {notice}
          </Notice>
        )}
        {board && project ? (
          <div className={`board-host${loading ? " loading" : ""}`}>
            <Board
              project={project}
              tasks={tasks}
              openId={openId}
              draggable={!isTouch}
              onOpen={(id) => setOpenId(id === openId ? null : id)}
              onMove={moveCard}
              onAdd={addTo}
              chips={(t) => <TaskChips task={t} fields={project.fields} />}
            />
          </div>
        ) : (
          <div className={`tasks-list${loading ? " loading" : ""}`}>
            {tasks.length === 0 && !loading && <div className="placeholder">Nothing here.</div>}
            {tasks.map((t) => (
              <TaskRow
                key={t.id}
                task={t}
                project={sel.kind === "smart" ? byId.get(t.project_id) : undefined}
                status={statusOf(t)}
                fields={byId.get(t.project_id)?.fields}
                selected={t.id === openId}
                onToggle={() => void toggleDone(t)}
                onOpen={() => setOpenId(t.id === openId ? null : t.id)}
              />
            ))}
          </div>
        )}
        {!board && pages > 1 && (
          <div className="tasks-pager">
            <button type="button" className="btn sm" disabled={page <= 1} onClick={() => setPage(page - 1)} title="Previous page" aria-label="Previous page">
              <Icon name="chevron-left" size={14} />
            </button>
            <span>
              {page} / {pages}
            </span>
            <button type="button" className="btn sm" disabled={page >= pages} onClick={() => setPage(page + 1)} title="Next page" aria-label="Next page">
              <Icon name="chevron-right" size={14} />
            </button>
          </div>
        )}
      </main>

      {open && (
        <TaskDetail
          key={open.id}
          task={open}
          projects={projects}
          statuses={byId.get(open.project_id)?.statuses ?? []}
          fields={byId.get(open.project_id)?.fields ?? []}
          labels={labels}
          onChange={replaceOrMove}
          onLabels={setLabels}
          onClose={() => setOpenId(null)}
          onDelete={() => remove(open)}
          onError={setError}
          canSend={agents.length > 0}
          onSend={(groom) => setSendReq({ task: open, groom })}
          canStart={machines.some((m) => m.state === "connected" && m.sessions.some((s) => s.dir))}
          onStart={() => setStartReq(open)}
          machineLabel={(id) => machineLabel(machines, id)}
          liveStatus={(r) => agents.find((a) => a.machineId === r.machine && a.paneId === r.pane)?.status ?? null}
          onOpenPane={onOpenPane}
        />
      )}
      {startReq && (
        <StartAgentDialog
          task={startReq}
          project={byId.get(startReq.project_id)}
          moveTo={byId.get(startReq.project_id)?.statuses.find((s) => s.on_start)?.title}
          machines={machines}
          currentPane={currentPane}
          onClose={() => setStartReq(null)}
          onStarted={(t, where) => {
            setStartReq(null);
            replace(t);
            announce(`Started ${t.run?.agent ?? "an agent"} on ${t.run?.branch ?? "a worktree"} — ${where}. The task follows once it is up.`, {
              machineId: t.run?.machine ?? "",
              paneId: t.run?.pane ?? "",
            });
          }}
        />
      )}
      {sendReq && (
        <SendToAgent
          title={sendReq.groom ? "Send for grooming" : "Send to agent"}
          text={taskPrompt(sendReq.task, byId.get(sendReq.task.project_id), sendReq.groom)}
          agents={agents}
          currentPane={currentPane}
          rememberKey={`task-project:${sendReq.task.project_id}`}
          onClose={() => setSendReq(null)}
          onSend={async (target, text) => {
            await onSend(target.machineId, target.paneId, text);
            setSendReq(null);
            announce(`Sent “${sendReq.task.title}” to ${target.agent} in ${target.sessionName}${target.machineId !== "local" ? ` on ${target.machineLabel}` : ""}.`, target);
          }}
        />
      )}
      {newProject && (
        <NewProjectDialog
          projects={projects}
          onClose={() => setNewProject(false)}
          onCreated={(p) => {
            setNewProject(false);
            setProjects((ps) => [...(ps ?? []), p]);
            select({ kind: "project", id: p.id });
          }}
        />
      )}
      {settings && project && (
        <ProjectSettings
          project={project}
          onClose={() => setSettings(false)}
          onSaved={(p) => {
            setSettings(false);
            setProjects((ps) => (ps ?? []).map((x) => (x.id === p.id ? p : x)));
            reloadList();
          }}
        />
      )}
    </>
  );
}

// ---- sidebar ----

function ProjectTree({ projects, selected, onSelect }: { projects: Project[]; selected: number; onSelect: (id: number) => void }) {
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
          {p.is_favorite && <Icon name="star" size={12} className="tasks-star" />}
        </button>
        {render(children.get(p.id) ?? [], depth + 1)}
      </div>
    ));
  return <div className="tasks-tree">{render(roots, 0)}</div>;
}

function NewProjectDialog({ projects, onClose, onCreated }: { projects: Project[]; onClose: () => void; onCreated: (p: Project) => void }) {
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

// ---- list ----

function QuickAdd({ placeholder, disabled, onAdd }: { placeholder: string; disabled: boolean; onAdd: (title: string) => Promise<void> }) {
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

function TaskRow({ task: t, project, status, fields, selected, onToggle, onOpen }: {
  task: Task;
  project?: Project;
  status?: Status;
  fields?: Field[];
  selected: boolean;
  onToggle: () => void;
  onOpen: () => void;
}) {
  return (
    <div
      className={`tasks-row${t.done ? " done" : ""}${selected ? " sel" : ""}`}
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
      <TaskChips task={t} project={project} status={status} fields={fields} />
    </div>
  );
}

// TaskChips is what a task shows beside its title, on a list row and a board card alike
function TaskChips({ task: t, project, status, fields }: { task: Task; project?: Project; status?: Status; fields?: Field[] }) {
  const due = dueLabel(t);
  return (
    <>
      {(fields ?? []).filter((f) => f.on_card).map((f) => (
        <FieldChip key={f.id} field={f} value={t.fields?.[String(f.id)]} />
      ))}
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

// ---- detail ----

function TaskDetail({ task: t, projects, statuses, fields, labels, onChange, onLabels, onClose, onDelete, onError, canSend, onSend, canStart, onStart, machineLabel, liveStatus, onOpenPane }: {
  task: Task;
  projects: Project[];
  statuses: Status[];
  fields: Field[];
  labels: Label[];
  onChange: (t: Task) => void;
  onLabels: (ls: Label[]) => void;
  onClose: () => void;
  onDelete: () => void;
  onError: (msg: string) => void;
  canSend: boolean;
  onSend: (groom: boolean) => void;
  canStart: boolean;
  onStart: () => void;
  machineLabel: (id: string) => string;
  liveStatus: (r: Run) => string | null; // the pane's status from the roster, null when it is not there
  onOpenPane: (machineId: string, paneId: string) => void;
}) {
  const [comment, setComment] = useState("");
  const [commenting, setCommenting] = useState(false);
  const comments = t.comments ?? [];
  const run = t.run ?? null;
  const paneStatus = run ? liveStatus(run) : null;
  const runLabel = !run ? "" : run.state === "starting" ? "starting…" : run.state === "failed" ? "failed" : run.state === "gone" ? "closed" : paneStatus === "run" ? "working" : paneStatus === "input" ? "waiting for you" : paneStatus === null ? "closed" : run.state === "finished" ? "finished" : "running";
  const postComment = async () => {
    const body = comment.trim();
    if (!body || commenting) return;
    setCommenting(true);
    try {
      const c = await addComment(t.id, body);
      onChange({ ...t, comments: [...comments, c] });
      setComment("");
    } catch (e) {
      onError(errMsg(e));
    } finally {
      setCommenting(false);
    }
  };
  const [title, setTitle] = useState(t.title);
  const [sendMenu, setSendMenu] = useState(false);
  const closeSendMenu = useCallback(() => setSendMenu(false), []);
  useDismiss(sendMenu, closeSendMenu, [".split"]);
  const [editingDesc, setEditingDesc] = useState(false);
  const [desc, setDesc] = useState("");
  const [labelPick, setLabelPick] = useState(false);
  const [labelQuery, setLabelQuery] = useState("");

  const patch = async (fields: Partial<Task>) => {
    try {
      onChange(await patchTask(t.id, fields));
    } catch (e) {
      onError(errMsg(e));
    }
  };
  const saveTitle = () => {
    const v = title.trim();
    if (v && v !== t.title) void patch({ title: v });
    else setTitle(t.title);
  };
  const addLabel = async (l: Label) => {
    setLabelPick(false);
    setLabelQuery("");
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
      onLabels([...labels, l]);
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
  const wanted = labelQuery.trim().toLowerCase();
  const candidates = labels.filter((l) => !have.has(l.id) && l.title.toLowerCase().includes(wanted));
  const exact = candidates.find((l) => l.title.toLowerCase() === wanted);
  const attached = (t.labels ?? []).some((l) => l.title.toLowerCase() === wanted);

  return (
    <aside className="tasks-detail">
      <div className="tasks-detail-head">
        <input
          type="checkbox"
          className="tasks-check"
          checked={t.done}
          onChange={() => void patch({ done: !t.done })}
          title={t.done ? "Mark undone" : "Mark done"}
          aria-label={t.done ? "Mark undone" : "Mark done"}
        />
        <span className="tasks-ident">{t.identifier}</span>
        <button type="button" className="icon-btn tasks-close" onClick={onClose} title="Close" aria-label="Close">
          <Icon name="x" size={16} />
        </button>
      </div>
      <input
        className="tasks-title"
        value={title}
        onChange={(e) => setTitle(e.target.value)}
        onBlur={saveTitle}
        onKeyDown={(e) => {
          if (e.key === "Enter") (e.target as HTMLInputElement).blur();
          if (e.key === "Escape") setTitle(t.title);
        }}
      />

      <div className="tasks-field">
        <span className="tasks-field-name">Status</span>
        <Select value={t.status_id} onChange={(status_id) => void patch({ status_id })} options={statuses.map(statusOption)} />
      </div>
      <div className="tasks-field">
        <span className="tasks-field-name">Project</span>
        <Select
          value={t.project_id}
          onChange={(project_id) => void patch({ project_id })}
          options={projects.filter((p) => !p.is_archived || p.id === t.project_id).map(projectOption)}
        />
      </div>
      <div className="tasks-field">
        <span className="tasks-field-name">Due</span>
        <DueInput value={t.due_date} onCommit={(iso) => void patch({ due_date: iso })} />
        {t.due_date && (
          <button type="button" className="linkish" onClick={() => void patch({ due_date: "" })}>
            clear
          </button>
        )}
      </div>
      <div className="tasks-field">
        <span className="tasks-field-name">Priority</span>
        <Select
          value={t.priority}
          onChange={(priority) => void patch({ priority })}
          options={PRIORITIES.map((p, i) => ({ value: i, label: p, icon: <PrioMark level={i} /> }))}
        />
      </div>
      <div className="tasks-field labels">
        <span className="tasks-field-name">Labels</span>
        <span className="tasks-labels">
          {(t.labels ?? []).map((l) => (
            <span key={l.id} className="tasks-label" style={labelStyle(l)}>
              {l.title}
              <button type="button" className="icon-btn sm tasks-label-x" onClick={() => void dropLabel(l)} title="Remove" aria-label={`Remove ${l.title}`}>
                <Icon name="x" size={12} />
              </button>
            </span>
          ))}
          {labelPick ? (
            <span className="tasks-label-pick">
              <input
                autoFocus
                value={labelQuery}
                placeholder="label…"
                onChange={(e) => setLabelQuery(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === "Escape") setLabelPick(false);
                  if (e.key === "Enter") {
                    e.preventDefault();
                    const q = labelQuery.trim();
                    if (exact) void addLabel(exact);
                    else if (!q && candidates[0]) void addLabel(candidates[0]);
                    else if (q && !attached) void newLabel(q);
                  }
                }}
                onBlur={() => setTimeout(() => setLabelPick(false), 150)}
              />
              <span className="row-menu label-menu" role="listbox">
                {candidates.slice(0, 8).map((l) => (
                  <button key={l.id} type="button" onMouseDown={(e) => e.preventDefault()} onClick={() => void addLabel(l)}>
                    <span className="tasks-dot" style={{ background: `#${l.hex_color || "888"}` }} /> {l.title}
                  </button>
                ))}
                {labelQuery.trim() && !exact && !attached && (
                  <button type="button" onMouseDown={(e) => e.preventDefault()} onClick={() => void newLabel(labelQuery)}>
                    <Icon name="plus" size={12} /> create “{labelQuery.trim()}”
                  </button>
                )}
              </span>
            </span>
          ) : (
            <button type="button" className="linkish" onClick={() => setLabelPick(true)}>
              <Icon name="plus" size={12} /> label
            </button>
          )}
        </span>
      </div>
      {fields.map((f) => (
        <div key={f.id} className={`tasks-field kind-${f.kind}`}>
          <span className="tasks-field-name" title={f.in_prompt ? "Goes to the agent with the task" : undefined}>
            {f.title}
          </span>
          <FieldInput field={f} value={t.fields?.[String(f.id)]} onCommit={(v) => void patch({ fields: { [String(f.id)]: v } })} />
        </div>
      ))}

      <div className="tasks-send">
        <span className={`split${canSend ? "" : " off"}`} title={canSend ? "" : "No agent is running in herdr"}>
          <button type="button" className="split-main" disabled={!canSend} onClick={() => onSend(false)}>
            Send to agent
          </button>
          <button type="button" className="split-arrow" disabled={!canSend} onClick={() => setSendMenu((m) => !m)} title="More ways to send" aria-haspopup="menu" aria-expanded={sendMenu}>
            <Icon name="chevron-down" size={14} />
          </button>
          {sendMenu && (
            <span className="row-menu split-menu" role="menu">
              <button type="button" role="menuitem" onClick={() => { setSendMenu(false); onSend(false); }}>
                Send to agent
              </button>
              <button type="button" role="menuitem" onClick={() => { setSendMenu(false); onSend(true); }}>
                Send to agent for grooming
                <span className="split-hint">/grill-me</span>
              </button>
              <button type="button" role="menuitem" disabled={!canStart} onClick={() => { setSendMenu(false); onStart(); }} title={canStart ? "" : "No connected machine has a workspace to branch off"}>
                Start an agent in a new worktree…
              </button>
            </span>
          )}
        </span>
        {!run && canStart && (
          <button type="button" className="linkish" onClick={onStart}>
            <Icon name="git-branch" size={12} /> Start an agent on it
          </button>
        )}
      </div>

      {run && (
        <div className={`tasks-run is-${run.state}`}>
          <span className="tasks-run-logo">
            <Logo agent={run.agent} />
          </span>
          <span className="tasks-run-copy">
            <span className="tasks-run-title">
              {run.agent} on <code>{run.branch}</code>
              {run.machine !== "local" && <> · {machineLabel(run.machine)}</>}
            </span>
            <span className="tasks-run-sub">
              {runLabel}
              {run.error && <> — {run.error}</>}
            </span>
          </span>
          {paneStatus !== null && (
            <button type="button" className="linkish" onClick={() => onOpenPane(run.machine, run.pane)}>
              Open <Icon name="chevron-right" size={12} />
            </button>
          )}
        </div>
      )}

      <div className="tasks-desc">
        <div className="tasks-field-name">
          Description
          {!editingDesc && (
            <button
              type="button"
              className="linkish"
              onClick={() => {
                setDesc(htmlToText(t.description));
                setEditingDesc(true);
              }}
            >
              <Icon name="edit" size={12} /> edit
            </button>
          )}
        </div>
        {editingDesc ? (
          <>
            <textarea value={desc} onChange={(e) => setDesc(e.target.value)} rows={8} autoFocus />
            <div className="modal-actions">
              <button type="button" className="btn" onClick={() => setEditingDesc(false)}>
                Cancel
              </button>
              <button
                type="button"
                className="btn primary"
                onClick={() => {
                  setEditingDesc(false);
                  void patch({ description: textToHtml(desc) });
                }}
              >
                Save
              </button>
            </div>
          </>
        ) : t.description ? (
          <div className="tasks-html">
            <Html html={t.description} />
          </div>
        ) : (
          <div className="placeholder small">No description.</div>
        )}
      </div>

      <div className="tasks-comments">
        <div className="tasks-field-name">Comments</div>
        {comments.map((c) => (
          <div key={c.id} className={`tasks-comment by-${c.author}`}>
            <div className="tasks-comment-head">
              <span className="tasks-comment-who">
                {c.author === "agent" ? (
                  <>
                    <Logo agent={run?.agent ?? "claude"} /> {run?.agent ?? "agent"}
                  </>
                ) : c.author === "you" ? (
                  <>
                    <Icon name="user" size={12} /> you
                  </>
                ) : (
                  <>
                    <Icon name="info" size={12} /> remuda
                  </>
                )}
              </span>
              <span className="tasks-comment-when" title={new Date(c.created).toLocaleString()}>
                {whenShort(c.created)}
              </span>
              <button
                type="button"
                className="icon-btn sm"
                title="Delete comment"
                aria-label="Delete comment"
                onClick={() =>
                  void deleteComment(c.id).then(
                    () => onChange({ ...t, comments: comments.filter((x) => x.id !== c.id) }),
                    (e) => onError(errMsg(e)),
                  )
                }
              >
                <Icon name="x" size={12} />
              </button>
            </div>
            <div className="tasks-comment-body">
              <Markdown text={c.body} onOpenFile={() => {}} />
            </div>
          </div>
        ))}
        {run?.state === "finished" && !t.done && (
          <button type="button" className="btn primary tasks-comment-done" onClick={() => void patch({ done: true })}>
            <Icon name="check" size={14} /> Mark done
          </button>
        )}
        <div className="tasks-comment-add">
          <textarea
            rows={2}
            value={comment}
            placeholder="A note on this task…"
            onChange={(e) => setComment(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) void postComment();
            }}
          />
          <button type="button" className="btn" disabled={commenting || !comment.trim()} onClick={() => void postComment()}>
            Comment
          </button>
        </div>
      </div>

      <div className="tasks-detail-foot">
        <span className="tasks-meta">
          created {fmtDate(t.created)}
          {t.done && t.done_at && <> · done {fmtDate(t.done_at)}</>}
        </span>
        <button type="button" className="linkish danger" onClick={onDelete}>
          <Icon name="trash" size={12} /> Delete
        </button>
      </div>
    </aside>
  );
}

// ---- send to an agent ----

// taskPrompt is what the agent gets — the same words the server composes (internal/tasks/prompt.go) — with /grill-me in front for a grooming pass
export function taskPrompt(t: Task, project: Project | undefined, groom: boolean): string {
  const desc = t.description ? htmlToText(t.description) : "";
  const fields = (project?.fields ?? [])
    .filter((f) => f.in_prompt)
    .map((f) => ({ f, text: valueText(f, t.fields?.[String(f.id)]) }))
    .filter((x) => x.text)
    .map((x) => `${x.f.title}: ${x.text}`)
    .join("\n");
  const parts = [t.title.trim(), desc, fields, `(remuda task ${t.identifier}${project ? ` in ${project.title}` : ""})`].filter(Boolean);
  return (groom ? "/grill-me " : "") + parts.join("\n\n");
}

// ---- start an agent on a task ----

// a branch name from a title: "task-12-fix-the-login-page"
function branchFor(t: Task): string {
  const slug = t.title
    .toLowerCase()
    .replace(/[^\p{L}\p{N}]+/gu, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 40)
    .replace(/-+$/g, "");
  return `task-${t.id}${slug ? `-${slug}` : ""}`;
}

function StartAgentDialog({ task: t, project, moveTo, machines, currentPane, onClose, onStarted }: {
  task: Task;
  project: Project | undefined;
  moveTo?: string;
  machines: Machine[];
  currentPane: { machineId: string; paneId: string } | null;
  onClose: () => void;
  onStarted: (t: Task, where: string) => void;
}) {
  const connected = machines.filter((m) => m.state === "connected" && m.sessions.some((s) => s.dir));
  const [remembered, setRemembered] = usePref<Record<string, { machine: string; workspace: string }>>("tasks.start", {});
  const last = remembered[String(t.project_id)];
  const [machine, setMachine] = useState(() => (connected.find((m) => m.id === last?.machine) ?? connected.find((m) => m.id === currentPane?.machineId) ?? connected[0])?.id ?? "");
  const workspaces = useMemo(() => {
    const m = machines.find((x) => x.id === machine);
    // the repos themselves, not their worktrees: a worktree branches off its repo
    return (m?.sessions ?? []).filter((s) => s.dir && !s.worktree?.linked).map((s) => ({ value: s.id, label: s.name, hint: s.dir, icon: <Icon name="folder" size={14} /> }));
  }, [machines, machine]);
  const [workspace, setWorkspace] = useState(() => (workspaces.find((w) => w.value === last?.workspace) ?? workspaces[0])?.value ?? "");
  useEffect(() => {
    if (!workspaces.some((w) => w.value === workspace)) setWorkspace(workspaces[0]?.value ?? "");
  }, [workspaces, workspace]);
  const [branch, setBranch] = useState(branchFor(t));
  const [base, setBase] = useState("");
  const [groom, setGroom] = useState(false);
  const [prompt, setPrompt] = useState(() => taskPrompt(t, project, false));
  const [edited, setEdited] = useState(false);
  const toggleGroom = (on: boolean) => {
    setGroom(on);
    // an edited prompt keeps its words; only the prefix comes and goes
    setPrompt((p) => (edited ? (on ? (p.startsWith("/grill-me") ? p : `/grill-me ${p}`) : p.replace(/^\/grill-me\s*/, "")) : taskPrompt(t, project, on)));
  };
  const ws = workspaces.find((w) => w.value === workspace);
  const m = machines.find((x) => x.id === machine);
  const ready = !!workspace && !!branch.trim() && !!prompt.trim();
  const { busy, err, run } = useSubmit(async () => {
    setRemembered({ ...remembered, [String(t.project_id)]: { machine, workspace } });
    const task = await startTask(t.id, { machine, workspace, branch: branch.trim(), base: base.trim(), agent: "claude", groom, prompt });
    onStarted(task, `${ws?.label ?? "the workspace"}${m && m.id !== "local" ? ` on ${m.label}` : ""}`);
  });
  return (
    <Dialog
      title={<>Start an agent on {t.identifier}</>}
      icon="git-branch"
      size="md"
      as="form"
      className="tasks-start"
      onClose={onClose}
      onSubmit={(e) => {
        e.preventDefault();
        if (ready && !busy) void run();
      }}
    >
      <p className="modal-hint">
        A worktree branches off the workspace's repository, Claude starts in it and gets the task once it is up.{" "}
        {moveTo ? (
          <>
            The task moves to <b>{moveTo}</b>;
          </>
        ) : (
          "The task stays where it is;"
        )}{" "}
        every answer the agent finishes comes back here as a comment.
      </p>
      <div className="schedule-row">
        {connected.length > 1 && (
          <label>
            Machine
            <Select value={machine} onChange={setMachine} options={machineOptions(connected, { onlyConnected: false })} />
          </label>
        )}
        <label className="grow">
          Workspace
          <Select value={workspace} onChange={setWorkspace} placeholder="Pick a repository" options={workspaces} />
        </label>
      </div>
      <div className="schedule-row">
        <label className="grow">
          Branch
          <input value={branch} onChange={(e) => setBranch(e.target.value)} spellCheck={false} required />
        </label>
        <label className="grow">
          From
          <input value={base} onChange={(e) => setBase(e.target.value)} placeholder="the current branch" spellCheck={false} />
        </label>
      </div>
      <label className="tasks-toggle">
        <input type="checkbox" checked={groom} onChange={(e) => toggleGroom(e.target.checked)} /> groom first (<code>/grill-me</code>)
      </label>
      <label>
        Prompt
        <textarea
          rows={6}
          value={prompt}
          onChange={(e) => {
            setPrompt(e.target.value);
            setEdited(true);
          }}
        />
      </label>
      {err && <Notice tone="error">{err}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          Cancel
        </button>
        <button type="submit" className="btn primary" disabled={busy || !ready}>
          {busy ? "Starting…" : "Start"}
        </button>
      </div>
    </Dialog>
  );
}

// DueInput keeps the value while typed and sends it on blur or Enter, never while empty (the clear button unsets):
// a datetime-local field fires a change per segment typed and reads as "" while incomplete
function DueInput({ value, onCommit }: { value: string; onCommit: (iso: string) => void }) {
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

// ---- pieces ----

const projectOption = (p: Project) => ({
  value: p.id,
  label: p.title,
  icon: <span className="tasks-dot" style={{ background: p.hex_color ? `#${p.hex_color}` : "var(--line-strong)" }} />,
});

const statusOption = (s: Status) => ({
  value: s.id,
  label: s.title,
  hint: s.kind === "done" ? "done" : undefined,
  icon: <span className="tasks-dot" style={{ background: `#${s.hex_color || "888"}` }} />,
});

// the priority's mark: nothing, then exclamation marks that redden
function PrioMark({ level }: { level: number }) {
  if (level === 0) return <span className="sel-glyph dim">—</span>;
  return <span className={`tasks-prio p${level}`}>{"!".repeat(Math.max(1, level - 1))}</span>;
}

// ---- helpers ----

function endOfToday(): string {
  const d = new Date();
  d.setHours(18, 0, 0, 0);
  return d.toISOString();
}

// "Today 18:00", "Tomorrow", "Thu", "12 Oct", "12 Oct 2027"
function dueLabel(t: Task): { text: string; over: boolean } | null {
  if (!t.due_date) return null;
  const d = new Date(t.due_date);
  const now = new Date();
  const day = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
  const days = Math.round((day(d) - day(now)) / 86400000);
  const time = d.getHours() || d.getMinutes() ? ` ${d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}` : "";
  let text: string;
  if (days === 0) text = `Today${time}`;
  else if (days === 1) text = `Tomorrow${time}`;
  else if (days === -1) text = "Yesterday";
  else if (days > 1 && days < 7) text = d.toLocaleDateString([], { weekday: "short" });
  else text = d.toLocaleDateString([], { day: "numeric", month: "short", year: d.getFullYear() === now.getFullYear() ? undefined : "numeric" });
  return { text, over: d.getTime() < now.getTime() };
}

// a Date's local wall-clock for <input type=datetime-local>
function toLocalInput(s: string): string {
  if (!s) return "";
  const d = new Date(s);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
}

function labelStyle(l: Label): CSSProperties {
  const hex = l.hex_color || "888888";
  return { background: `#${hex}33`, borderColor: `#${hex}88` };
}

// Descriptions are HTML (paragraphs; older ones may carry more): rendered through an allow-list into
// React nodes, never innerHTML, and edited as plain text that goes back as paragraphs.
const BLOCK = new Set(["p", "div", "li", "h1", "h2", "h3", "h4", "pre", "blockquote", "tr"]);

export function htmlToText(html: string): string {
  const doc = new DOMParser().parseFromString(html, "text/html");
  let out = "";
  const walk = (n: Node) => {
    if (n.nodeType === Node.TEXT_NODE) {
      out += n.textContent;
      return;
    }
    if (n.nodeType !== Node.ELEMENT_NODE) return;
    const tag = (n as Element).tagName.toLowerCase();
    if (tag === "br") {
      out += "\n";
      return;
    }
    if (tag === "li") out += "- ";
    n.childNodes.forEach(walk);
    if (BLOCK.has(tag)) out += "\n";
  };
  doc.body.childNodes.forEach(walk);
  return out.replace(/\n{3,}/g, "\n\n").trim();
}

export function textToHtml(text: string): string {
  const esc = (s: string) => s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
  return text
    .split(/\n{2,}/)
    .map((para) => `<p>${esc(para).replace(/\n/g, "<br>")}</p>`)
    .join("");
}

const ALLOWED = new Set(["p", "br", "strong", "b", "em", "i", "u", "s", "a", "ul", "ol", "li", "code", "pre", "h1", "h2", "h3", "h4", "blockquote", "hr", "input", "span", "div", "label"]);

function Html({ html }: { html: string }) {
  const nodes = useMemo(() => {
    const doc = new DOMParser().parseFromString(html, "text/html");
    let k = 0;
    const render = (n: Node): ReactNode => {
      if (n.nodeType === Node.TEXT_NODE) return n.textContent;
      if (n.nodeType !== Node.ELEMENT_NODE) return null;
      const el = n as Element;
      const tag = el.tagName.toLowerCase();
      const kids = Array.from(el.childNodes).map(render);
      const key = k++;
      if (!ALLOWED.has(tag)) return <span key={key}>{kids}</span>;
      if (tag === "a") {
        const href = el.getAttribute("href") ?? "";
        if (!/^https?:\/\//.test(href)) return <span key={key}>{kids}</span>;
        return (
          <a
            key={key}
            href={href}
            target="_blank"
            rel="noopener noreferrer"
            onClick={(e) => {
              e.preventDefault();
              openUrl(href);
            }}
          >
            {kids}
          </a>
        );
      }
      if (tag === "input") return <input key={key} type="checkbox" checked={el.hasAttribute("checked")} readOnly />;
      if (tag === "br" || tag === "hr") return createElement(tag, { key });
      return createElement(tag, { key }, ...kids);
    };
    return Array.from(doc.body.childNodes).map(render);
  }, [html]);
  return <>{nodes}</>;
}
