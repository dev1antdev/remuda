// remuda's own tasks: /api/tasks/* over the SQLite store on the remuda
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
  run?: Run | null; // the latest agent started on it
  comments?: Comment[]; // on a single-task fetch
};

// an agent started on a task: a worktree off a workspace, the agent in it
export type Run = {
  id: number;
  task_id: number;
  machine: string;
  workspace: string;
  pane: string;
  dir: string;
  branch: string;
  agent: string;
  state: "starting" | "running" | "finished" | "gone" | "failed";
  started: string;
  prompt_sent?: string;
  finished?: string;
  error?: string;
};

export type Comment = { id: number; task_id: number; author: "agent" | "you" | "remuda"; body: string; created: string };

export const PRIORITIES = ["None", "Low", "Medium", "High", "Urgent", "DO NOW"];

const get = <T,>(path: string): Promise<T> =>
  fetch(path)
    .then(check)
    .then((r) => r.json() as Promise<T>);

export const listProjects = () => get<Project[]>("/api/tasks/projects");
export const createProject = (title: string, parent_project_id = 0) => sendJSON<Project>("POST", "/api/tasks/projects", { title, parent_project_id });

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

export const startTask = (id: number, req: { machine: string; workspace: string; branch: string; base: string; agent: string; groom: boolean; prompt: string }) =>
  sendJSON<Task>("POST", `/api/tasks/${id}/start`, req);
export const addComment = (id: number, body: string) => sendJSON<Comment>("POST", `/api/tasks/${id}/comments`, { body });
export const deleteComment = (id: number) => sendJSON<unknown>("DELETE", `/api/tasks/comments/${id}`);
