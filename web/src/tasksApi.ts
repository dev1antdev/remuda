// Vikunja through herdr-ui: /api/tasks/v1/<path> is Vikunja's own REST API
// with the user's token added server-side. Shapes are Vikunja's, trimmed to
// what the UI reads. Dates are RFC 3339; an unset one is "0001-01-01…".

import { check, sendJSON } from "./api";

export type TasksStatus = {
  configured: boolean;
  url?: string;
  version?: string;
  error?: string;
};

export type Label = { id: number; title: string; hex_color: string };

export type ProjectView = {
  id: number;
  title: string;
  view_kind: "list" | "gantt" | "table" | "kanban";
};

export type Project = {
  id: number;
  title: string;
  description: string;
  identifier: string;
  hex_color: string;
  parent_project_id: number;
  is_archived: boolean;
  is_favorite: boolean;
  position: number;
  views: ProjectView[];
};

export type Task = {
  id: number;
  title: string;
  description: string; // HTML from Vikunja's editor
  done: boolean;
  done_at: string;
  due_date: string;
  start_date: string;
  end_date: string;
  priority: number; // 0 unset … 5 "do now"
  percent_done: number;
  project_id: number;
  bucket_id: number;
  labels: Label[] | null;
  identifier: string; // "#12" or "PROJ-12"
  index: number;
  is_favorite: boolean;
  created: string;
  updated: string;
};

export const NO_DATE = "0001-01-01T00:00:00Z";
export const hasDate = (d: string | undefined | null): d is string => !!d && !d.startsWith("0001-");

export const PRIORITIES = ["—", "Low", "Medium", "High", "Urgent", "DO NOW"];

export const tasksStatus = (checkNow = false) =>
  fetch(`/api/tasks/status${checkNow ? "?check=1" : ""}`)
    .then(check)
    .then((r) => r.json() as Promise<TasksStatus>);

export const tasksConnect = (url: string, token: string) =>
  fetch("/api/tasks/config", {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ url, token }),
  })
    .then(check)
    .then((r) => r.json() as Promise<TasksStatus>);

export const tasksDisconnect = () => sendJSON<TasksStatus>("DELETE", "/api/tasks/config");

type Query = Record<string, string | number | boolean | undefined | (string | number)[]>;

function qs(query?: Query): string {
  if (!query) return "";
  const p = new URLSearchParams();
  for (const [k, v] of Object.entries(query)) {
    if (v === undefined || v === "") continue;
    if (Array.isArray(v)) v.forEach((x) => p.append(k, String(x)));
    else p.set(k, String(v));
  }
  const s = p.toString();
  return s ? `?${s}` : "";
}

async function vk<T>(method: "GET" | "PUT" | "POST" | "PATCH" | "DELETE", path: string, body?: unknown, query?: Query): Promise<{ data: T; pages: number }> {
  const res = await check(
    await fetch(`/api/tasks/v1/${path}${qs(query)}`, {
      method,
      headers: body === undefined ? undefined : { "Content-Type": "application/json" },
      body: body === undefined ? undefined : JSON.stringify(body),
    }),
  );
  const text = await res.text();
  return { data: (text ? JSON.parse(text) : undefined) as T, pages: Number(res.headers.get("X-Pagination-Total-Pages") || 1) };
}

// Vikunja pages everything at its configured maximum (50 by default); the
// pseudo-projects (saved filters, favorites) ride along on every page, so
// ids are deduplicated
async function all<T extends { id: number }>(path: string, query?: Query): Promise<T[]> {
  const out = new Map<number, T>();
  for (let page = 1; page <= 20; page++) {
    const { data, pages } = await vk<T[]>("GET", path, undefined, { ...query, per_page: 50, page });
    for (const item of data ?? []) if (!out.has(item.id)) out.set(item.id, item);
    if (page >= pages) break;
  }
  return [...out.values()];
}

export const listProjects = () => all<Project>("projects");
export const listLabels = () => all<Label>("labels");

export type TaskQuery = {
  filter: string; // Vikunja's filter language: "done = false && project = 3"
  search?: string;
  sort?: { by: string; order: "asc" | "desc" }[];
  page?: number;
  timezone?: string;
};

export async function listTasks(q: TaskQuery): Promise<{ tasks: Task[]; pages: number }> {
  const sort = q.sort ?? [{ by: "due_date", order: "asc" }, { by: "id", order: "desc" }];
  const { data, pages } = await vk<Task[]>("GET", "tasks", undefined, {
    filter: q.filter,
    s: q.search,
    sort_by: sort.map((s) => s.by),
    order_by: sort.map((s) => s.order),
    filter_timezone: q.timezone,
    per_page: 50,
    page: q.page ?? 1,
  });
  return { tasks: data ?? [], pages };
}

// a saved filter is a pseudo-project with a negative id; its tasks come
// through its list view (the view carries the filter)
export async function listViewTasks(projectId: number, viewId: number, q: Omit<TaskQuery, "filter">): Promise<{ tasks: Task[]; pages: number }> {
  const sort = q.sort ?? [{ by: "due_date", order: "asc" }, { by: "id", order: "desc" }];
  const { data, pages } = await vk<Task[]>("GET", `projects/${projectId}/views/${viewId}/tasks`, undefined, {
    s: q.search,
    sort_by: sort.map((s) => s.by),
    order_by: sort.map((s) => s.order),
    filter_timezone: q.timezone,
    per_page: 50,
    page: q.page ?? 1,
  });
  return { tasks: data ?? [], pages };
}

export const getTask = (id: number) => vk<Task>("GET", `tasks/${id}`).then((r) => r.data);

export const createTask = (projectId: number, fields: Partial<Task> & { title: string }) =>
  vk<Task>("PUT", `projects/${projectId}/tasks`, fields).then((r) => r.data);

// PATCH is herdr-ui's: the server merges into the current task, since
// Vikunja's update replaces the whole thing
export const patchTask = (id: number, fields: Partial<Task>) => vk<Task>("PATCH", `tasks/${id}`, fields).then((r) => r.data);

export const deleteTask = (id: number) => vk<void>("DELETE", `tasks/${id}`);

export const addTaskLabel = (taskId: number, labelId: number) => vk<void>("PUT", `tasks/${taskId}/labels`, { label_id: labelId });
export const removeTaskLabel = (taskId: number, labelId: number) => vk<void>("DELETE", `tasks/${taskId}/labels/${labelId}`);
export const createLabel = (title: string, hex_color: string) => vk<Label>("PUT", "labels", { title, hex_color }).then((r) => r.data);

export const createProject = (title: string, parent_project_id = 0) =>
  vk<Project>("PUT", "projects", { title, parent_project_id }).then((r) => r.data);

export type CurrentUser = { id: number; username: string; name: string; settings?: { default_project_id?: number; timezone?: string } };
export const currentUser = () => vk<CurrentUser>("GET", "user").then((r) => r.data);
