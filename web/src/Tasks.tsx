// The Tasks mode: the user's Vikunja projects and tasks in herdr-ui's own
// clothes. Projects on the left (with three smart lists above them), the
// list in the middle, the open task on the right. Nothing is cached beyond
// the screen: every change goes to Vikunja and the list is re-read.

import { createElement, useCallback, useEffect, useMemo, useState, type CSSProperties, type ReactNode } from "react";
import {
  addTaskLabel,
  createLabel,
  createProject,
  createTask,
  currentUser,
  deleteTask,
  hasDate,
  listLabels,
  listProjects,
  listTasks,
  listViewTasks,
  NO_DATE,
  patchTask,
  PRIORITIES,
  removeTaskLabel,
  tasksConnect,
  tasksDisconnect,
  tasksStatus,
  type Label,
  type Project,
  type Task,
  type TasksStatus,
} from "./tasksApi";
import { errMsg, usePref } from "./prefs";
import { openUrl } from "./host";
import Select from "./Select";
import Logo from "./Logo";
import Icon, { type IconName } from "./icons";

type Smart = "today" | "upcoming" | "open";
type Sel = { kind: "smart"; id: Smart } | { kind: "project"; id: number };

const SMART: { id: Smart; label: string; icon: IconName; filter: string }[] = [
  { id: "today", label: "Today", icon: "sun", filter: "done = false && due_date < now/d+1d" },
  { id: "upcoming", label: "Upcoming", icon: "calendar", filter: "done = false && due_date < now+7d" },
  { id: "open", label: "All open", icon: "circle-check", filter: "done = false" },
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

export default function Tasks({ drawer, closeDrawer, ask, agents, currentPane, onOpenPane, onSend }: {
  drawer: boolean;
  closeDrawer: () => void;
  ask: (text: string, label: string, run: () => void) => void;
  agents: AgentTarget[];
  currentPane: { machineId: string; paneId: string } | null;
  onOpenPane: (machineId: string, paneId: string) => void;
  onSend: (machineId: string, paneId: string, text: string) => Promise<void>;
}) {
  const [status, setStatus] = useState<TasksStatus | null>(null);
  const [projects, setProjects] = useState<Project[]>([]);
  const [labels, setLabels] = useState<Label[]>([]);
  const [defaultProject, setDefaultProject] = useState(0);
  const [sel, setSel] = usePref<Sel>("tasks.sel", { kind: "smart", id: "today" });
  const [showDone, setShowDone] = usePref("tasks.showDone", false);
  const [search, setSearch] = useState("");
  const [tasks, setTasks] = useState<Task[]>([]);
  const [pages, setPages] = useState(1);
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");
  const [openId, setOpenId] = useState<number | null>(null);
  const [newProject, setNewProject] = useState(false);
  const [sendReq, setSendReq] = useState<{ task: Task; groom: boolean } | null>(null);
  const [notice, setNotice] = useState<{ text: string; machineId: string; paneId: string } | null>(null);
  const [tick, setTick] = useState(0);
  const refresh = useCallback(() => setTick((t) => t + 1), []);

  useEffect(() => {
    void tasksStatus().then(setStatus, (e) => setStatus({ configured: false, error: errMsg(e) }));
  }, []);

  // the sidebar's data, once connected
  const connected = !!status?.configured;
  useEffect(() => {
    if (!connected) return;
    let gone = false;
    void Promise.all([listProjects(), listLabels(), currentUser().catch(() => null)]).then(
      ([ps, ls, me]) => {
        if (gone) return;
        setProjects(ps);
        setLabels(ls);
        setDefaultProject(me?.settings?.default_project_id || ps.find((p) => p.title === "Inbox")?.id || ps[0]?.id || 0);
      },
      (e) => !gone && setError(errMsg(e)),
    );
    return () => {
      gone = true;
    };
  }, [connected, tick]);

  // the list: selection + done + search → one Vikunja query
  const filter = useMemo(() => {
    if (sel.kind === "smart") return SMART.find((s) => s.id === sel.id)!.filter;
    if (sel.id < 0) return `view ${sel.id}`; // a saved filter: its view decides
    return `project = ${sel.id}${showDone ? "" : " && done = false"}`;
  }, [sel, showDone]);
  const debounced = useDebounced(search, 250);
  const isFilter = sel.kind === "project" && sel.id < 0;
  const withDone = showDone && sel.kind === "project" && !isFilter;
  useEffect(() => {
    setPage(1);
  }, [filter, debounced]);
  useEffect(() => {
    if (!connected) return;
    let gone = false;
    setLoading(true);
    const sort = withDone
      ? [{ by: "done", order: "asc" as const }, { by: "due_date", order: "asc" as const }, { by: "id", order: "desc" as const }]
      : undefined;
    const savedFilter = isFilter ? projects.find((p) => p.id === sel.id) : undefined;
    const listView = savedFilter?.views.find((v) => v.view_kind === "list")?.id;
    const req = isFilter
      ? listView
        ? listViewTasks(sel.id, listView, { search: debounced, page, timezone: TZ })
        : Promise.resolve({ tasks: [], pages: 1 })
      : listTasks({ filter, search: debounced, page, timezone: TZ, sort });
    void req.then(
      (r) => {
        if (gone) return;
        setTasks(r.tasks);
        setPages(r.pages);
        if (page > r.pages) setPage(Math.max(1, r.pages)); // the page emptied out
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
  }, [connected, filter, debounced, page, tick, withDone, isFilter, sel.id, projects]);

  const byId = useMemo(() => new Map(projects.map((p) => [p.id, p])), [projects]);
  const open = openId === null ? null : (tasks.find((t) => t.id === openId) ?? null);

  // one task changed on the server: show it without re-reading the list
  const replace = (t: Task) => setTasks((ts) => ts.map((x) => (x.id === t.id ? t : x)));

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

  const remove = (t: Task) =>
    ask(`Delete “${t.title}”? This removes it from Vikunja.`, "Delete", () => {
      void deleteTask(t.id).then(() => {
        setOpenId(null);
        refresh();
      }, (e) => setError(errMsg(e)));
    });

  if (status === null) {
    return (
      <main className="pane-area">
        <div className="placeholder">Loading…</div>
      </main>
    );
  }
  if (!status.configured) {
    return (
      <main className="pane-area">
        <ConnectForm status={status} onConnected={setStatus} />
      </main>
    );
  }

  const addTarget = sel.kind === "project" && !isFilter ? byId.get(sel.id) : byId.get(defaultProject);
  const savedFilters = projects.filter((p) => p.id < 0);
  const title = sel.kind === "smart" ? SMART.find((s) => s.id === sel.id)!.label : (byId.get(sel.id)?.title ?? "…");

  return (
    <>
      <aside className={drawer ? "sidebar open" : "sidebar"}>
        <div className="sidebar-head">
          <span>Lists</span>
          <button onClick={() => setNewProject(true)} title="New project">
            <Icon name="plus" size={13} /> Project
          </button>
        </div>
        {SMART.map((s) => (
          <button
            key={s.id}
            className={`tasks-nav${sel.kind === "smart" && sel.id === s.id ? " sel" : ""}`}
            onClick={() => {
              setSel({ kind: "smart", id: s.id });
              setOpenId(null);
              closeDrawer();
            }}
          >
            <span className="tasks-nav-glyph">
              <Icon name={s.icon} size={15} />
            </span>
            {s.label}
          </button>
        ))}
        {savedFilters.length > 0 && (
          <>
            <div className="sidebar-head">
              <span>Filters</span>
            </div>
            {savedFilters.map((f) => (
              <button
                key={f.id}
                className={`tasks-nav${sel.kind === "project" && sel.id === f.id ? " sel" : ""}`}
                onClick={() => {
                  setSel({ kind: "project", id: f.id });
                  setOpenId(null);
                  closeDrawer();
                }}
                title={f.description || f.title}
              >
                <span className="tasks-nav-glyph">
                  <Icon name="filter" size={14} />
                </span>
                <span className="tasks-nav-label">{f.title}</span>
              </button>
            ))}
          </>
        )}
        <div className="sidebar-head">
          <span>Projects</span>
        </div>
        <ProjectTree
          projects={projects.filter((p) => p.id > 0)}
          selected={sel.kind === "project" ? sel.id : 0}
          onSelect={(id) => {
            setSel({ kind: "project", id });
            setOpenId(null);
            closeDrawer();
          }}
        />
        <div className="tasks-sidebar-foot">
          <button className="linkish" onClick={() => openUrl(status.url!)} title={status.url}>
            Open Vikunja <Icon name="external-link" size={12} />
          </button>
          <button
            className="linkish"
            onClick={() =>
              ask(`Disconnect from ${status.url}? The token is forgotten on this herdr-ui; your tasks stay in Vikunja.`, "Disconnect", () => {
                void tasksDisconnect().then(setStatus);
              })
            }
          >
            Disconnect
          </button>
        </div>
      </aside>
      {drawer && <div className="scrim" onClick={closeDrawer} />}

      <main className="pane-area tasks-main">
        <div className="tasks-head">
          <h1>{title}</h1>
          {sel.kind === "project" && !isFilter && (
            <label className="tasks-toggle">
              <input type="checkbox" checked={showDone} onChange={(e) => setShowDone(e.target.checked)} /> done
            </label>
          )}
          <input className="tasks-search" type="search" placeholder="Search…" value={search} onChange={(e) => setSearch(e.target.value)} />
        </div>
        <QuickAdd placeholder={addTarget ? `Add a task to ${addTarget.title}…` : "Add a task…"} disabled={!addTarget} onAdd={addTask} />
        {error && <div className="chat-error">{error}</div>}
        {notice && (
          <div className="tasks-notice">
            {notice.text}
            <button className="linkish" onClick={() => onOpenPane(notice.machineId, notice.paneId)}>
              Open <Icon name="chevron-right" size={12} />
            </button>
            <button className="tasks-close icon-btn" onClick={() => setNotice(null)} title="Dismiss">
              <Icon name="x" size={14} />
            </button>
          </div>
        )}
        <div className={`tasks-list${loading ? " loading" : ""}`}>
          {tasks.length === 0 && !loading && <div className="placeholder">Nothing here.</div>}
          {tasks.map((t) => (
            <TaskRow
              key={t.id}
              task={t}
              project={sel.kind === "smart" || isFilter ? byId.get(t.project_id) : undefined}
              selected={t.id === openId}
              onToggle={() => void toggleDone(t)}
              onOpen={() => setOpenId(t.id === openId ? null : t.id)}
            />
          ))}
        </div>
        {pages > 1 && (
          <div className="tasks-pager">
            <button disabled={page <= 1} onClick={() => setPage(page - 1)}>
              <Icon name="chevron-left" size={14} />
            </button>
            <span>
              {page} / {pages}
            </span>
            <button disabled={page >= pages} onClick={() => setPage(page + 1)}>
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
          labels={labels}
          vikunjaURL={status.url!}
          onChange={replace}
          onLabels={setLabels}
          onClose={() => setOpenId(null)}
          onDelete={() => remove(open)}
          onError={setError}
          canSend={agents.length > 0}
          onSend={(groom) => setSendReq({ task: open, groom })}
        />
      )}
      {sendReq && (
        <SendDialog
          task={sendReq.task}
          groom={sendReq.groom}
          vikunjaURL={status.url!}
          agents={agents}
          currentPane={currentPane}
          onClose={() => setSendReq(null)}
          onSend={async (target, text) => {
            await onSend(target.machineId, target.paneId, text);
            setSendReq(null);
            setNotice({
              text: `Sent “${sendReq.task.title}” to ${target.agent} in ${target.sessionName}${target.machineId !== "local" ? ` on ${target.machineLabel}` : ""}.`,
              machineId: target.machineId,
              paneId: target.paneId,
            });
          }}
        />
      )}
      {newProject && (
        <NewProjectDialog
          projects={projects}
          onClose={() => setNewProject(false)}
          onCreated={(p) => {
            setNewProject(false);
            setProjects((ps) => [...ps, p]);
            setSel({ kind: "project", id: p.id });
          }}
        />
      )}
    </>
  );
}

// ---- connect ----

function ConnectForm({ status, onConnected }: { status: TasksStatus; onConnected: (s: TasksStatus) => void }) {
  const [url, setUrl] = useState(status.url ?? "");
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState(status.error ?? "");
  return (
    <form
      className="tasks-connect"
      onSubmit={(e) => {
        e.preventDefault();
        setBusy(true);
        setErr("");
        tasksConnect(url, token).then(onConnected, (e) => setErr(errMsg(e))).finally(() => setBusy(false));
      }}
    >
      <h2>Connect Vikunja</h2>
      <p className="modal-hint">
        Your tasks stay in your own Vikunja; herdr-ui only talks to its API with a token you create there under{" "}
        <b>Settings → API Tokens</b>. Give it <code>projects</code>, <code>projects_views_tasks</code>, <code>tasks</code>,{" "}
        <code>tasks_labels</code>, <code>labels</code> and <code>other: user</code>. The token is stored on this herdr-ui, never sent to a browser.
      </p>
      <label>
        Vikunja URL
        <input value={url} onChange={(e) => setUrl(e.target.value)} placeholder="https://todo.example.com" autoFocus required />
      </label>
      <label>
        API token
        <input value={token} onChange={(e) => setToken(e.target.value)} placeholder="tk_…" type="password" required />
      </label>
      {err && <div className="chat-error">{err}</div>}
      <div className="modal-actions">
        <button type="submit" className="primary" disabled={busy}>
          {busy ? "…" : "Connect"}
        </button>
      </div>
    </form>
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
  const ids = new Set(projects.map((p) => p.id));
  // roots: no parent, or a parent we cannot see
  const roots = [...(children.get(0) ?? []), ...[...children.entries()].filter(([pid]) => pid !== 0 && !ids.has(pid)).flatMap(([, l]) => l)];
  const render = (list: Project[], depth: number): ReactNode =>
    list.map((p) => (
      <div key={p.id}>
        <button
          className={`tasks-nav project${p.id === selected ? " sel" : ""}`}
          style={{ paddingLeft: 12 + depth * 14 }}
          onClick={() => onSelect(p.id)}
          title={p.description || p.title}
        >
          <span className="tasks-dot" style={{ background: p.hex_color ? `#${p.hex_color}` : "var(--line-strong)" }} />
          <span className="tasks-nav-label">{p.title}</span>
          {p.is_favorite && <span className="tasks-star">★</span>}
        </button>
        {render(children.get(p.id) ?? [], depth + 1)}
      </div>
    ));
  return <div className="tasks-tree">{render(roots, 0)}</div>;
}

function NewProjectDialog({ projects, onClose, onCreated }: { projects: Project[]; onClose: () => void; onCreated: (p: Project) => void }) {
  const [title, setTitle] = useState("");
  const [parent, setParent] = useState(0);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  return (
    <div className="modal-scrim" onClick={onClose}>
      <form
        className="modal"
        onClick={(e) => e.stopPropagation()}
        onSubmit={(e) => {
          e.preventDefault();
          setBusy(true);
          createProject(title.trim(), parent).then(onCreated, (e) => {
            setErr(errMsg(e));
            setBusy(false);
          });
        }}
      >
        <h2>New project</h2>
        <label>
          Name
          <input value={title} onChange={(e) => setTitle(e.target.value)} autoFocus required />
        </label>
        <label>
          Inside
          <Select
            value={parent}
            onChange={setParent}
            options={[{ value: 0, label: "— top level —" }, ...projects.filter((p) => p.id > 0 && !p.is_archived).map(projectOption)]}
          />
        </label>
        {err && <div className="chat-error">{err}</div>}
        <div className="modal-actions">
          <button type="button" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" className="primary" disabled={busy || !title.trim()}>
            Create
          </button>
        </div>
      </form>
    </div>
  );
}

// ---- list ----

function QuickAdd({ placeholder, disabled, onAdd }: { placeholder: string; disabled: boolean; onAdd: (title: string) => Promise<void> }) {
  const [value, setValue] = useState("");
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  return (
    <form
      className="tasks-add"
      onSubmit={(e) => {
        e.preventDefault();
        const title = value.trim();
        if (!title || busy) return;
        setBusy(true);
        onAdd(title).then(
          () => {
            setValue("");
            setErr("");
            setBusy(false);
          },
          (e) => {
            setErr(errMsg(e));
            setBusy(false);
          },
        );
      }}
    >
      <span className="tasks-add-plus">
        <Icon name="plus" size={15} />
      </span>
      <input value={value} onChange={(e) => setValue(e.target.value)} placeholder={placeholder} disabled={disabled || busy} />
      {err && <span className="chat-error">{err}</span>}
    </form>
  );
}

function TaskRow({ task: t, project, selected, onToggle, onOpen }: {
  task: Task;
  project?: Project;
  selected: boolean;
  onToggle: () => void;
  onOpen: () => void;
}) {
  const due = dueLabel(t);
  return (
    <div className={`tasks-row${t.done ? " done" : ""}${selected ? " sel" : ""}`} onClick={onOpen}>
      <input
        type="checkbox"
        className="tasks-check"
        checked={t.done}
        onChange={onToggle}
        onClick={(e) => e.stopPropagation()}
        title={t.done ? "Mark undone" : "Mark done"}
      />
      <span className="tasks-row-title">{t.title}</span>
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
      {project && <span className="tasks-row-project">{project.title}</span>}
    </div>
  );
}

// ---- detail ----

function TaskDetail({ task: t, projects, labels, vikunjaURL, onChange, onLabels, onClose, onDelete, onError, canSend, onSend }: {
  task: Task;
  projects: Project[];
  labels: Label[];
  vikunjaURL: string;
  onChange: (t: Task) => void;
  onLabels: (ls: Label[]) => void;
  onClose: () => void;
  onDelete: () => void;
  onError: (msg: string) => void;
  canSend: boolean;
  onSend: (groom: boolean) => void;
}) {
  const [title, setTitle] = useState(t.title);
  const [sendMenu, setSendMenu] = useState(false);
  useEffect(() => {
    if (!sendMenu) return;
    const close = () => setSendMenu(false);
    document.addEventListener("click", close);
    return () => document.removeEventListener("click", close);
  }, [sendMenu]);
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
  const candidates = labels.filter((l) => !have.has(l.id) && l.title.toLowerCase().includes(labelQuery.toLowerCase()));
  const exact = candidates.find((l) => l.title.toLowerCase() === labelQuery.trim().toLowerCase());
  const attached = (t.labels ?? []).some((l) => l.title.toLowerCase() === labelQuery.trim().toLowerCase());

  return (
    <aside className="tasks-detail">
      <div className="tasks-detail-head">
        <input type="checkbox" className="tasks-check" checked={t.done} onChange={() => void patch({ done: !t.done })} />
        <span className="tasks-ident">{t.identifier || `#${t.index}`}</span>
        <button className="linkish" onClick={() => openUrl(`${vikunjaURL}/tasks/${t.id}`)} title="Open this task in Vikunja">
          Vikunja <Icon name="external-link" size={12} />
        </button>
        <button className="tasks-close icon-btn" onClick={onClose} title="Close">
          <Icon name="x" size={15} />
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
        <span className="tasks-field-name">Project</span>
        <Select
          value={t.project_id}
          onChange={(project_id) => void patch({ project_id })}
          options={projects.filter((p) => p.id > 0 && (!p.is_archived || p.id === t.project_id)).map(projectOption)}
        />
      </div>
      <div className="tasks-field">
        <span className="tasks-field-name">Due</span>
        <DueInput value={t.due_date} onCommit={(iso) => void patch({ due_date: iso })} />
        {hasDate(t.due_date) && (
          <button className="linkish" onClick={() => void patch({ due_date: NO_DATE })}>
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
              <button onClick={() => void dropLabel(l)} title="Remove">
                <Icon name="x" size={11} />
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
              <span className="row-menu label-menu">
                {candidates.slice(0, 8).map((l) => (
                  <button key={l.id} onMouseDown={(e) => e.preventDefault()} onClick={() => void addLabel(l)}>
                    <span className="tasks-dot" style={{ background: `#${l.hex_color || "888"}` }} /> {l.title}
                  </button>
                ))}
                {labelQuery.trim() && !exact && !attached && (
                  <button onMouseDown={(e) => e.preventDefault()} onClick={() => void newLabel(labelQuery.trim())}>
                    + create “{labelQuery.trim()}”
                  </button>
                )}
              </span>
            </span>
          ) : (
            <button className="linkish" onClick={() => setLabelPick(true)}>
              + label
            </button>
          )}
        </span>
      </div>

      <div className="tasks-send">
        <span className={`split${canSend ? "" : " off"}`} title={canSend ? "" : "No agent is running in herdr"}>
          <button className="split-main" disabled={!canSend} onClick={() => onSend(false)}>
            Send to agent
          </button>
          <button
            className="split-arrow"
            disabled={!canSend}
            onClick={(e) => {
              e.stopPropagation();
              setSendMenu((m) => !m);
            }}
            title="More ways to send"
          >
            <Icon name="chevron-down" size={14} />
          </button>
          {sendMenu && (
            <span className="row-menu split-menu" onClick={(e) => e.stopPropagation()}>
              <button onClick={() => { setSendMenu(false); onSend(false); }}>Send to agent</button>
              <button onClick={() => { setSendMenu(false); onSend(true); }}>
                Send to agent for grooming
                <span className="split-hint">/grill-me</span>
              </button>
            </span>
          )}
        </span>
      </div>

      <div className="tasks-desc">
        <div className="tasks-field-name">
          Description
          {!editingDesc && (
            <button
              className="linkish"
              onClick={() => {
                setDesc(htmlToText(t.description));
                setEditingDesc(true);
              }}
            >
              edit
            </button>
          )}
        </div>
        {editingDesc ? (
          <>
            <textarea value={desc} onChange={(e) => setDesc(e.target.value)} rows={8} autoFocus />
            <div className="modal-actions">
              <button onClick={() => setEditingDesc(false)}>Cancel</button>
              <button
                className="primary"
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

      <div className="tasks-detail-foot">
        <span className="tasks-meta">
          created {fmtDate(t.created)}
          {t.done && hasDate(t.done_at) && <> · done {fmtDate(t.done_at)}</>}
        </span>
        <button className="linkish danger" onClick={onDelete}>
          <Icon name="trash" size={12} /> Delete
        </button>
      </div>
    </aside>
  );
}

// ---- send to an agent ----

// taskPrompt is what the agent gets: the title, the description as text,
// and where the task lives — with /grill-me in front for a grooming pass.
export function taskPrompt(t: Task, vikunjaURL: string, groom: boolean): string {
  const desc = t.description ? htmlToText(t.description) : "";
  const parts = [t.title.trim(), desc, `Task ${t.identifier || `#${t.index}`} in Vikunja: ${vikunjaURL}/tasks/${t.id}`].filter(Boolean);
  return (groom ? "/grill-me " : "") + parts.join("\n\n");
}

const targetKey = (a: { machineId: string; paneId: string }) => `${a.machineId}|${a.paneId}`;

function SendDialog({ task, groom, vikunjaURL, agents, currentPane, onClose, onSend }: {
  task: Task;
  groom: boolean;
  vikunjaURL: string;
  agents: AgentTarget[];
  currentPane: { machineId: string; paneId: string } | null;
  onClose: () => void;
  onSend: (target: AgentTarget, text: string) => Promise<void>;
}) {
  // the last agent each project's tasks went to, so the next one is one click
  const [remembered, setRemembered] = usePref<Record<string, string>>("tasks.targets", {});
  const initial =
    agents.find((a) => targetKey(a) === remembered[task.project_id]) ??
    (currentPane && agents.find((a) => targetKey(a) === targetKey(currentPane))) ??
    agents[0];
  const [target, setTarget] = useState(initial ? targetKey(initial) : "");
  const [text, setText] = useState(() => taskPrompt(task, vikunjaURL, groom));
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const chosen = agents.find((a) => targetKey(a) === target);
  return (
    <div className="modal-scrim" onClick={onClose}>
      <form
        className="modal tasks-send-dialog"
        onClick={(e) => e.stopPropagation()}
        onSubmit={(e) => {
          e.preventDefault();
          if (!chosen || !text.trim()) return;
          setBusy(true);
          setErr("");
          onSend(chosen, text).then(
            () => setRemembered({ ...remembered, [task.project_id]: target }),
            (e) => {
              setErr(errMsg(e));
              setBusy(false);
            },
          );
        }}
      >
        <h2>{groom ? "Send for grooming" : "Send to agent"}</h2>
        <label>
          Agent
          <Select
            value={target}
            onChange={setTarget}
            options={agents.map((a) => ({
              value: targetKey(a),
              label: `${a.sessionName} › ${a.paneTitle}`,
              hint: `${a.agent} · ${a.status}`,
              group: a.machineLabel,
              icon: <Logo agent={a.agent} />,
            }))}
          />
        </label>
        {chosen && <p className="modal-hint tasks-send-cwd">{chosen.cwd}</p>}
        <label>
          Message
          <textarea value={text} onChange={(e) => setText(e.target.value)} rows={groom ? 10 : 8} />
        </label>
        {err && <div className="chat-error">{err}</div>}
        <div className="modal-actions">
          <button type="button" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" className="primary" disabled={busy || !chosen || !text.trim()}>
            {busy ? "…" : "Send"}
          </button>
        </div>
      </form>
    </div>
  );
}

// A datetime-local field fires a change per segment typed (and reads as ""
// while incomplete), so the value is kept here and sent when the user is
// done: on blur or Enter, never while empty (the clear button unsets).
function DueInput({ value, onCommit }: { value: string; onCommit: (iso: string) => void }) {
  const [local, setLocal] = useState(toLocalInput(value));
  useEffect(() => setLocal(toLocalInput(value)), [value]);
  const commit = () => {
    if (!local) return setLocal(toLocalInput(value));
    const iso = new Date(local).toISOString();
    if (iso !== (hasDate(value) ? new Date(value).toISOString() : "")) onCommit(iso);
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

// the priority's mark: nothing, then exclamation marks that redden
function PrioMark({ level }: { level: number }) {
  if (level === 0) return <span className="sel-glyph dim">—</span>;
  return <span className={`tasks-prio p${level}`}>{"!".repeat(Math.max(1, level - 1))}</span>;
}

// ---- helpers ----

function useDebounced<T>(value: T, ms: number): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const id = setTimeout(() => setV(value), ms);
    return () => clearTimeout(id);
  }, [value, ms]);
  return v;
}

function endOfToday(): string {
  const d = new Date();
  d.setHours(18, 0, 0, 0);
  return d.toISOString();
}

// "Today 18:00", "Tomorrow", "Thu", "12 Oct", "12 Oct 2027"
function dueLabel(t: Task): { text: string; over: boolean } | null {
  if (!hasDate(t.due_date)) return null;
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

const fmtDate = (s: string) => new Date(s).toLocaleDateString([], { day: "numeric", month: "short", year: "numeric" });

// a Date's local wall-clock for <input type=datetime-local>
function toLocalInput(s: string): string {
  if (!hasDate(s)) return "";
  const d = new Date(s);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
}

function labelStyle(l: Label): CSSProperties {
  const hex = l.hex_color || "888888";
  return { background: `#${hex}33`, borderColor: `#${hex}88` };
}

const PALETTE = ["e67e22", "2ecc71", "3498db", "9b59b6", "e74c3c", "1abc9c", "f1c40f", "95a5a6"];
const randomColor = () => PALETTE[Math.floor(Math.random() * PALETTE.length)]!;

// Vikunja descriptions are HTML from its editor. Shown through a tiny
// allow-list renderer (React nodes, never innerHTML); edited as plain text,
// which goes back as paragraphs.
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
