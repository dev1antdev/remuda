// The triage of what agents propose: which column an accepted task goes to, how many wait, the words for them.

import { plural } from "@/lib/text";
import type { Project, Status } from "./api";

// acceptColumn is where an accepted proposal goes: the project's new-task column, else its first open column that is not the proposals' own
export function acceptColumn(project: Project | undefined): Status | undefined {
  if (!project) return undefined;
  return project.statuses.find((s) => s.on_new) ?? project.statuses.find((s) => s.kind !== "done" && !s.on_agent);
}

// proposedTotal counts the proposals waiting across the projects shown
export function proposedTotal(projects: Project[]): number {
  return projects.reduce((n, p) => n + (p.is_archived ? 0 : p.proposed || 0), 0);
}

// proposedWords says how many wait: "3 tasks proposed by agents", "1 task proposed by an agent"
export function proposedWords(n: number): string {
  return n === 1 ? "1 task proposed by an agent" : `${plural(n, "task")} proposed by agents`;
}
