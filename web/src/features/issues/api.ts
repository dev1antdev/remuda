// The issues panel's calls: a machine's GitHub repositories, the list a search finds, one issue whole, and the writes
// the person presses. Each goes to gh on the machine through the server, as the person.

import { getJSON, sendJSON } from "@/lib/http";

export type Person = { login: string; bot?: boolean };
export type Label = { name: string; color: string; description?: string };
export type Trouble = { kind: "no-gh" | "signed-out" | "not-found" | "failed"; text: string; run?: string };

export type Item = {
  number: number;
  title: string;
  author: Person;
  state: "open" | "closed";
  reason?: "completed" | "not_planned";
  labels: Label[];
  assignees: Person[];
  updated: string;
  url: string;
};

export type List = { items: Item[]; more: boolean; me: string; at: string; source: string };

export type Reaction = { content: string; count: number };
export type Ref = { repo: string; number: number; title: string; state: string; url: string; pull?: boolean; merged?: boolean };
export type Event = {
  kind: "comment" | "labels" | "assign" | "closed" | "reopened" | "renamed" | "milestone" | "mention" | "commit";
  actor: Person;
  at: string;
  body?: string;
  role?: string;
  url?: string;
  reactions?: Reaction[];
  added?: Label[];
  removed?: Label[];
  who?: string;
  on?: boolean;
  reason?: string;
  from?: string;
  to?: string;
  title?: string;
  ref?: Ref;
  sha?: string;
};
export type Pull = { repo: string; number: number; title: string; url: string; state: string; draft?: boolean };

export type Detail = {
  number: number;
  title: string;
  body: string;
  url: string;
  state: "open" | "closed";
  reason?: "completed" | "not_planned";
  author: Person;
  role?: string;
  created: string;
  updated: string;
  closed?: string;
  labels: Label[];
  assignees: Person[];
  milestone?: { title: string; url?: string };
  reactions: Reaction[];
  comments: number;
  timeline: Event[];
  pulls: Pull[];
  participants: Person[];
  pull?: boolean;
  me: string;
  at: string;
  source: string;
};

export type Link = { branch: string; number: number };
export type Repo = { repo: string; folder: string; dirs: string[]; links: Link[] };
export type Here = { dir: string; repo?: string; folder?: string; why?: "no-folder" | "no-git" | "no-origin" | "not-github"; host?: string };
export type Repos = { repos: Repo[]; here?: Here };

// a read gh could not answer comes back as its trouble, said with what to run
export type Answer<T> = T | { trouble: Trouble };

export function troubleOf<T extends object>(answer: Answer<T> | undefined): Trouble | null {
  return answer && "trouble" in answer ? answer.trouble : null;
}

const q = (fields: Record<string, string | number | boolean | undefined>) => {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(fields)) if (value !== undefined && value !== "" && value !== false) params.set(key, value === true ? "1" : String(value));
  return params.toString();
};

export const fetchRepos = (machine: string, dir?: string) => getJSON<Repos>(`/api/issues/repos?${q({ machine, dir })}`);

export const fetchIssues = (machine: string, repo: string, query: string, fresh = false) => getJSON<Answer<List>>(`/api/issues?${q({ machine, repo, q: query, fresh })}`);

export const fetchIssue = (machine: string, repo: string, number: number, fresh = false) => getJSON<Answer<Detail>>(`/api/issues/${number}?${q({ machine, repo, fresh })}`);

export const commentIssue = (machine: string, repo: string, number: number, body: string) => sendJSON("POST", `/api/issues/${number}/comment`, { machine, repo, body });

export const closeIssue = (machine: string, repo: string, number: number, reason: "completed" | "not_planned", comment = "") => sendJSON("POST", `/api/issues/${number}/close`, { machine, repo, reason, comment });

export const reopenIssue = (machine: string, repo: string, number: number, comment = "") => sendJSON("POST", `/api/issues/${number}/reopen`, { machine, repo, comment });

export const assignIssue = (machine: string, repo: string, number: number, me: boolean) => sendJSON("POST", `/api/issues/${number}/assign`, { machine, repo, me });
