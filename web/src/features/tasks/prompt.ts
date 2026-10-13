// What a task says outside its own screen: the prompt an agent gets, the branch it works on, its due date in words.

import type { Project, Task } from "./api";
import { valueText } from "./fields";
import { htmlToText } from "./html";

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

// branchFor is a branch name from the title: "task-12-fix-the-login-page"
export function branchFor(t: Pick<Task, "id" | "title">): string {
  const slug = t.title
    .toLowerCase()
    .replace(/[^\p{L}\p{N}]+/gu, "-")
    .replace(/^-+|-+$/g, "")
    .slice(0, 40)
    .replace(/-+$/g, "");
  return `task-${t.id}${slug ? `-${slug}` : ""}`;
}

export function endOfToday(now = new Date()): string {
  const d = new Date(now);
  d.setHours(18, 0, 0, 0);
  return d.toISOString();
}

// dueLabel: "Today 18:00", "Tomorrow", "Thu", "12 Oct", "12 Oct 2027"
export function dueLabel(t: Pick<Task, "due_date">, now = new Date()): { text: string; over: boolean } | null {
  if (!t.due_date) return null;
  const d = new Date(t.due_date);
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

// toLocalInput is a Date's local wall-clock for <input type=datetime-local>
export function toLocalInput(s: string): string {
  if (!s) return "";
  const d = new Date(s);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}T${p(d.getHours())}:${p(d.getMinutes())}`;
}
