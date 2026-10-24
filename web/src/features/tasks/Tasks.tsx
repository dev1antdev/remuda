// The Tasks mode: remuda's own task list (internal/tasks). Projects on the left with three smart lists
// above them, the list or board in the middle, the open task on the right. Nothing is cached beyond the
// screen: every change goes to the server and the list is re-read.

import { useEffect, useMemo, useRef, useState } from "react";
import { useCurrent } from "@/app/current";
import { actions, app } from "@/app/store";
import SendToAgent from "@/features/agents/SendToAgent";
import { useAgentTargets } from "@/features/agents/useAgentTargets";
import { submitPrompt } from "@/features/roster/api";
import { machineLabel } from "@/features/roster/options";
import { useRoster } from "@/features/roster/useRoster";
import { isTouch, usePref } from "@/lib/prefs";
import { useQuery } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { Icon, Notice, PANELS, Resizer, type IconName, useDebounced, useTimedNotice } from "@/ui";
import { createTask, deleteTask, getTask, listLabels, listProjects, listTasks, patchTask, type Task } from "./api";
import Board from "./Board";
import { Loading, TaskChips } from "./pieces";
import ProjectSettings from "./ProjectSettings";
import { NewProjectDialog, ProjectTree } from "./ProjectTree";
import { endOfToday, taskPrompt } from "./prompt";
import { StartAgentDialog } from "./StartAgentDialog";
import { TaskDetail } from "./TaskDetail";
import { QuickAdd, TaskRow } from "./TaskList";

type Smart = "today" | "upcoming" | "open";
type Sel = { kind: "smart"; id: Smart } | { kind: "project"; id: number };

// the smart lists: due by the end of today, due within a week, everything open
const SMART: { id: Smart; label: string; icon: IconName }[] = [
  { id: "today", label: "Today", icon: "sun" },
  { id: "upcoming", label: "Upcoming", icon: "calendar" },
  { id: "open", label: "All open", icon: "circle-check" },
];

const TZ = Intl.DateTimeFormat().resolvedOptions().timeZone;

export default function Tasks() {
  const drawer = app.use((s) => s.drawer);
  const { machines } = useRoster();
  const agents = useAgentTargets();
  const located = useCurrent();
  const currentPane = located ? { machineId: located.machine.id, paneId: located.pane.id } : null;
  const [sel, setSel] = usePref<Sel>("tasks.sel", { kind: "smart", id: "today" });
  const [showDone, setShowDone] = usePref("tasks.showDone", false);
  const [view, setView] = usePref<"list" | "board">("tasks.view", "board");
  const [settings, setSettings] = useState(false);
  const [search, setSearch] = useState("");
  const [page, setPage] = useState(1);
  const [actionError, setActionError] = useState("");
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

  const catalog = useQuery(() => Promise.all([listProjects(), listLabels()]).then(([projects, labels]) => ({ projects, labels })), []);
  const projects = catalog.data?.projects ?? null;
  const labels = catalog.data?.labels ?? [];
  const defaultProject = projects ? projects.find((p) => p.title === "Inbox" && !p.is_archived)?.id || projects.find((p) => !p.is_archived)?.id || 0 : 0;

  const board = sel.kind === "project" && view === "board";
  const withDone = showDone && sel.kind === "project" && !board;
  const query = useMemo(
    () => (sel.kind === "smart" ? { view: sel.id } : board ? { view: "board" as const, project: sel.id } : { view: "project" as const, project: sel.id, done: withDone }),
    [sel, withDone, board],
  );
  const debounced = useDebounced(search, 250);
  // the list stays on screen, dimmed, while the next one loads; the board re-reads itself every 15 s while on screen
  const shown = useRef<Task[]>([]);
  const list = useQuery<{ pages: number; tasks: Task[] }>(
    async () => {
      const r = await listTasks({ ...query, search: debounced, page, timezone: TZ });
      // the list knows no comments; an open task keeps the ones it loaded
      return { pages: r.pages, tasks: r.tasks.map((t) => ({ ...t, comments: t.comments ?? shown.current.find((p) => p.id === t.id)?.comments })) };
    },
    [query, debounced, page],
    { keep: true, every: board ? 15000 : undefined },
  );
  const tasks = list.data?.tasks ?? [];
  shown.current = tasks;
  const pages = list.data?.pages ?? 1;
  useEffect(() => {
    if (page > pages) setPage(Math.max(1, pages));
  }, [page, pages]);
  const replace = (t: Task) => list.mutate((d) => d && { ...d, tasks: d.tasks.map((x) => (x.id === t.id ? t : x)) });

  // the open task in full (its comments), re-read while an agent is on it
  const open = openId === null ? null : (tasks.find((t) => t.id === openId) ?? null);
  const live = !!open?.run && open.run.state !== "gone" && open.run.state !== "failed";
  const detail = useQuery(() => (openId === null ? Promise.resolve(undefined) : getTask(openId)), [openId], { enabled: openId !== null, every: live ? 10000 : undefined });
  useEffect(() => {
    if (detail.data) replace(detail.data);
  }, [detail.data]); // eslint-disable-line react-hooks/exhaustive-deps

  const refresh = () => {
    catalog.reload();
    list.reload();
    detail.reload();
  };
  const error = actionError || catalog.error || list.error;
  const select = (next: Sel) => {
    setSel(next);
    setPage(1);
    setOpenId(null);
    actions.closeDrawer();
  };

  const byId = useMemo(() => new Map((projects ?? []).map((p) => [p.id, p])), [projects]);
  const project = sel.kind === "project" ? byId.get(sel.id) : undefined;
  const statusOf = (t: Task) => byId.get(t.project_id)?.statuses.find((s) => s.id === t.status_id);

  // replaceOrMove re-reads the list when the task left it — another project, another due date — or changed column, where the server knows the place
  const replaceOrMove = (t: Task) => {
    const was = tasks.find((x) => x.id === t.id);
    replace(t);
    if (was && (was.project_id !== t.project_id || was.due_date !== t.due_date)) refresh();
    else if (was && was.status_id !== t.status_id) list.reload();
  };
  const toggleDone = async (t: Task) => {
    const want = !t.done;
    replace({ ...t, done: want });
    try {
      replace(await patchTask(t.id, { done: want }));
    } catch (e) {
      replace(t);
      setActionError(errMsg(e));
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
    list.reload();
  };
  // a dropped card is shown there at once; the server is told and the list put back as it was if it refuses
  const moveCard = (t: Task, statusId: number, after: number | undefined) => {
    const before = tasks;
    list.mutate((d) => {
      if (!d) return d;
      const rest = d.tasks.filter((x) => x.id !== t.id);
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
      return { ...d, tasks: rest };
    });
    void patchTask(t.id, { status_id: statusId, after }).then(replace, (e) => {
      list.mutate((d) => d && { ...d, tasks: before });
      setActionError(errMsg(e));
    });
  };
  const remove = (t: Task) =>
    actions.ask(`Delete “${t.title}”?`, "Delete", () => {
      void deleteTask(t.id).then(() => {
        setOpenId(null);
        refresh();
      }, (e) => setActionError(errMsg(e)));
    });

  if (projects === null) {
    return (
      <main className="pane-area">
        {error ? (
          <Notice tone="error" icon="alert-triangle" className="tasks-notice">
            {error}
          </Notice>
        ) : (
          <Loading />
        )}
      </main>
    );
  }

  const addTarget = sel.kind === "project" ? byId.get(sel.id) : byId.get(defaultProject);
  const title = sel.kind === "smart" ? SMART.find((s) => s.id === sel.id)!.label : (byId.get(sel.id)?.title ?? "…");

  return (
    <>
      <aside className={drawer ? "sidebar tasks-sidebar open" : "sidebar tasks-sidebar"}>
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
      <Resizer spec={PANELS.tasksSidebar} edge="right" label="Lists width" />
      {drawer && <div className="scrim" onClick={actions.closeDrawer} />}

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
          <Notice tone="error" icon="alert-triangle" className="tasks-notice" onDismiss={() => setActionError("")}>
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
                <button type="button" className="linkish" onClick={() => actions.selectPane(noticeAt.machineId, noticeAt.paneId)}>
                  Open <Icon name="chevron-right" size={12} />
                </button>
              )
            }
          >
            {notice}
          </Notice>
        )}
        {board && project ? (
          <div className={`board-host${list.fetching ? " loading" : ""}`}>
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
          <div className={`tasks-list${list.fetching ? " loading" : ""}`}>
            {tasks.length === 0 && !list.fetching && <div className="placeholder">Nothing here.</div>}
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

      {open && <Resizer spec={PANELS.taskDetail} edge="left" label="Task width" />}
      {open && (
        <TaskDetail
          key={open.id}
          task={open}
          projects={projects}
          statuses={byId.get(open.project_id)?.statuses ?? []}
          fields={byId.get(open.project_id)?.fields ?? []}
          labels={labels}
          onChange={replaceOrMove}
          onLabelsChanged={catalog.reload}
          onClose={() => setOpenId(null)}
          onDelete={() => remove(open)}
          onError={setActionError}
          canSend={agents.length > 0}
          onSend={(groom) => setSendReq({ task: open, groom })}
          canStart={machines.some((m) => m.state === "connected" && m.sessions.some((s) => s.dir))}
          onStart={() => setStartReq(open)}
          machineLabel={(id) => machineLabel(machines, id)}
          liveStatus={(r) => agents.find((a) => a.machineId === r.machine && a.paneId === r.pane)?.status ?? null}
          onOpenPane={actions.selectPane}
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
            await submitPrompt(target.machineId, target.paneId, text);
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
            catalog.mutate((d) => d && { ...d, projects: [...d.projects, p] });
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
            catalog.mutate((d) => d && { ...d, projects: d.projects.map((x) => (x.id === p.id ? p : x)) });
            list.reload();
          }}
        />
      )}
    </>
  );
}
