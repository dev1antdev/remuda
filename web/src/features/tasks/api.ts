// remuda's own tasks: /api/tasks/* over the SQLite store on the remuda
// host (internal/tasks). Dates are RFC 3339 UTC strings; an unset one is "".

import { check, sendJSON } from "@/lib/http";

export type Label = { id: number; title: string; hex_color: string };

// Status is one column of a project's board: a task is always in exactly one, and done follows the column's kind.
// The flags say where the workflow's own moves land — at most one column per flag.
export type Status = {
  id: number;
  project_id: number;
  title: string;
  hex_color: string;
  kind: "open" | "done";
  position: number;
  on_new: boolean;
  on_start: boolean;
  on_finish: boolean;
};

// a project's own field: what its tasks carry besides what every task has
export type FieldKind = "text" | "number" | "select" | "multi" | "date" | "check" | "link";
export type Field = {
  id: number;
  project_id: number;
  title: string;
  kind: FieldKind;
  options: string[];
  position: number;
  on_card: boolean;
  in_prompt: boolean;
};
export const FIELD_KINDS: { value: FieldKind; label: string }[] = [
  { value: "text", label: "Text" },
  { value: "number", label: "Number" },
  { value: "select", label: "One choice" },
  { value: "multi", label: "Several choices" },
  { value: "date", label: "Date" },
  { value: "check", label: "Checkbox" },
  { value: "link", label: "Link" },
];
// a task's value for a field; null clears it in a patch
export type FieldValue = string | number | boolean | string[] | null;

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
  statuses: Status[]; // the workflow, in order
  fields: Field[]; // its own fields, in order
};

export type Task = {
  id: number;
  project_id: number;
  status_id: number;
  title: string;
  description: string; // HTML: paragraphs from the editor
  done: boolean;
  done_at: string;
  due_date: string;
  priority: number; // 0 unset … 5 "do now"
  position: number;
  labels: Label[];
  fields: Record<string, FieldValue>; // by field id; no key, no value
  identifier: string; // "#12"
  source: string; // "" for a person's; an agent's names its key's machine and its pane, "GOG · w3:p1"
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
export const patchProject = (id: number, fields: Partial<Pick<Project, "title" | "description" | "hex_color" | "parent_project_id" | "is_archived" | "is_favorite" | "position">>) =>
  sendJSON<Project>("PATCH", `/api/tasks/projects/${id}`, fields);
// putStatuses replaces the workflow whole: a column with id 0 is new, one left out is removed and its
// tasks go to moves[old id], which the server insists on while it has any
export const putStatuses = (projectId: number, statuses: Status[], moves: Record<number, number>) =>
  sendJSON<Project>("PUT", `/api/tasks/projects/${projectId}/statuses`, { statuses, moves });
// the fields replaced whole: a field with id 0 is new, one left out goes with its values
export const putFields = (projectId: number, fields: Field[]) => sendJSON<Project>("PUT", `/api/tasks/projects/${projectId}/fields`, { fields });

export const listLabels = () => get<Label[]>("/api/tasks/labels");
export const createLabel = (title: string, hex_color: string) => sendJSON<Label>("POST", "/api/tasks/labels", { title, hex_color });

export type TaskQuery = {
  view: "today" | "upcoming" | "open" | "project" | "board"; // board: a project whole, in column order, one page
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
// patchTask: after is the task to sit behind in its column, 0 for the top; without it a task that changed column goes to the bottom
export const patchTask = (id: number, fields: Partial<Task> & { after?: number }) => sendJSON<Task>("PATCH", `/api/tasks/${id}`, fields);
export const deleteTask = (id: number) => sendJSON<unknown>("DELETE", `/api/tasks/${id}`);

export const addTaskLabel = (taskId: number, labelId: number) => sendJSON<Task>("PUT", `/api/tasks/${taskId}/labels`, { label_id: labelId });
export const removeTaskLabel = (taskId: number, labelId: number) => sendJSON<Task>("DELETE", `/api/tasks/${taskId}/labels/${labelId}`);

export const startTask = (id: number, req: { machine: string; workspace: string; branch: string; base: string; agent: string; groom: boolean; prompt: string }) =>
  sendJSON<Task>("POST", `/api/tasks/${id}/start`, req);
// importIssue makes a task of a GitHub issue, read with gh on the machine as the person
export const importIssue = (machine: string, projectId: number, url: string) => sendJSON<Task>("POST", "/api/tasks/import", { machine, project_id: projectId, url });
export const addComment = (id: number, body: string) => sendJSON<Comment>("POST", `/api/tasks/${id}/comments`, { body });
export const deleteComment = (id: number) => sendJSON<unknown>("DELETE", `/api/tasks/comments/${id}`);
