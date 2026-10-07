// herdr-ui's own tasks: /api/tasks/* over the SQLite store on the herdr-ui
// host (internal/tasks). Dates are RFC 3339 UTC strings; an unset one is "".

import { check, sendJSON } from "./api";

export type Label = { id: number; title: string; hex_color: string };

export type Project = {
  id: number;
  title: string;
  description: string;
  hex_color: string;
  parent_project_id: number;
  is_archived: boolean;
  is_favorite: boolean;
  position: number;
  created: string;
  updated: string;
};

export type Task = {
  id: number;
  project_id: number;
  title: string;
  description: string; // HTML: paragraphs from the editor
  done: boolean;
  done_at: string;
  due_date: string;
  priority: number; // 0 unset … 5 "do now"
  labels: Label[];
  identifier: string; // "#12"
  created: string;
  updated: string;
};

export const NO_DATE = "";
export const hasDate = (d: string | undefined | null): d is string => !!d;

export const PRIORITIES = ["None", "Low", "Medium", "High", "Urgent", "DO NOW"];

const get = <T,>(path: string): Promise<T> =>
  fetch(path)
    .then(check)
    .then((r) => r.json() as Promise<T>);

export const listProjects = () => get<Project[]>("/api/tasks/projects");
export const createProject = (title: string, parent_project_id = 0) => sendJSON<Project>("POST", "/api/tasks/projects", { title, parent_project_id });
export const patchProject = (id: number, fields: Partial<Project>) => sendJSON<Project>("PATCH", `/api/tasks/projects/${id}`, fields);
export const deleteProject = (id: number) => sendJSON<unknown>("DELETE", `/api/tasks/projects/${id}`);

export const listLabels = () => get<Label[]>("/api/tasks/labels");
export const createLabel = (title: string, hex_color: string) => sendJSON<Label>("POST", "/api/tasks/labels", { title, hex_color });

export type TaskQuery = {
  view: "today" | "upcoming" | "open" | "project";
  project?: number;
  done?: boolean; // include done tasks (project view)
  search?: string;
  page?: number;
  timezone?: string;
};

export async function listTasks(q: TaskQuery): Promise<{ tasks: Task[]; pages: number }> {
  const p = new URLSearchParams({ view: q.view });
  if (q.project) p.set("project", String(q.project));
  if (q.done) p.set("done", "1");
  if (q.search) p.set("q", q.search);
  if (q.page) p.set("page", String(q.page));
  if (q.timezone) p.set("tz", q.timezone);
  return get<{ tasks: Task[]; pages: number }>(`/api/tasks?${p}`);
}

export const getTask = (id: number) => get<Task>(`/api/tasks/${id}`);
export const createTask = (projectId: number, fields: Partial<Task> & { title: string }) => sendJSON<Task>("POST", "/api/tasks", { ...fields, project_id: projectId });
export const patchTask = (id: number, fields: Partial<Task>) => sendJSON<Task>("PATCH", `/api/tasks/${id}`, fields);
export const deleteTask = (id: number) => sendJSON<unknown>("DELETE", `/api/tasks/${id}`);

export const addTaskLabel = (taskId: number, labelId: number) => sendJSON<Task>("PUT", `/api/tasks/${taskId}/labels`, { label_id: labelId });
export const removeTaskLabel = (taskId: number, labelId: number) => sendJSON<Task>("DELETE", `/api/tasks/${taskId}/labels/${labelId}`);
