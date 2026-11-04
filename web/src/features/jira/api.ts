// The person's Jira site on the server: read with the token masked, set, removed, tested, and an issue made a task.

import { getJSON, sendJSON } from "@/lib/http";
import type { Task } from "@/features/tasks/api";

export type JiraView = { site: string; account: string; has_token: boolean; cloud: boolean };

export const fetchJira = () => getJSON<JiraView>("/api/jira");

// saveJira sets the site; an empty token keeps the one the server holds
export const saveJira = (site: string, account: string, token: string) => sendJSON<JiraView>("PUT", "/api/jira", { site, account, token });

export const removeJira = () => sendJSON<JiraView>("DELETE", "/api/jira");

export const testJira = () => sendJSON<{ ok: boolean; who: string; projects: number }>("POST", "/api/jira/test");

export const importJira = (projectId: number, key: string) => sendJSON<Task>("POST", "/api/jira/import", { project_id: projectId, key });
