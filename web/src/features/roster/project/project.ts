// A repository's project settings, after Orca's: where new worktrees start and go, the issue's branch pattern, the agent
// and first prompts they start with, the setup script and when it runs, and the run actions — named commands the ▶
// button starts in a shell. Kept on the cockpit by machine and main folder (internal/repos). Pure.

import type { IconName } from "@/ui/icons";
import { t } from "@/lib/i18n";

export type Place = "tab" | "right" | "down";
export type Action = { id: string; name: string; command: string; folder?: string; icon?: IconName; place?: Place };
export type SetupWhen = "" | "ask" | "run" | "skip";
export type Settings = {
  base?: string;
  location?: string;
  issue_branch?: string;
  agent?: string; // "" the last used, "shell" none
  issue_prompt?: string;
  pr_prompt?: string;
  setup?: string;
  setup_when?: SetupWhen;
  setup_wait?: boolean;
  actions?: Action[];
};
export type Defaults = { issue_branch: string; issue_prompt: string; pr_prompt: string };
// Project is GET /api/repos/settings: the settings and what the repository says of itself, read on its machine
export type Project = { root: string; name: string; settings: Settings; defaults: Defaults; icons: IconName[]; setup: boolean; base?: string; team?: { file: string; script: string } | null; trouble?: string };

export const DEFAULTS: Defaults = {
  issue_branch: "dev/feat/{number}",
  issue_prompt: "Work on GitHub issue #{number} in {repo}: “{title}”. Read it first with: gh issue view {number} -R {repo} --comments",
  pr_prompt: "Pick up GitHub pull request #{number} in {repo}: “{title}”. Read it first with: gh pr view {number} -R {repo} --comments",
};

export const PLACES: Place[] = ["tab", "right", "down"];
export const ICONS: IconName[] = ["play", "terminal", "circle-check", "sparkles", "layers", "settings", "zap", "refresh-cw", "monitor", "server", "code", "download"];

// VARIABLES are what a template fills in; HOOK_VARS what a setup script sees
export const VARIABLES = ["{number}", "{title}", "{repo}", "{branch}"];
export const HOOK_VARS: [string, string][] = [
  ["HERDR_ROOT_PATH", "The repository's main checkout"],
  ["HERDR_WORKTREE_PATH", "The new worktree's folder"],
  ["HERDR_WORKSPACE_NAME", "The new worktree's folder name"],
];

export function placeWords(place: Place | undefined): string {
  return place === "right" ? t("split right") : place === "down" ? t("split down") : t("a new tab");
}

// actionHint is a menu row's hint: the command and where it runs
export function actionHint(action: Action): string {
  return `${action.command}${action.folder ? ` · ${action.folder}/` : ""} · ${placeWords(action.place)}`;
}

// lastKey is the pref the run button keeps the last action used in, per repository and device
export const lastKey = (machine: string, root: string) => `run.last:${machine}|${root}`;

// primary is the action the button's main part runs: the last used, else the first
export function primary(actions: Action[] | undefined, last: string): Action | undefined {
  return (actions ?? []).find((a) => a.id === last) ?? actions?.[0];
}

// withAction is the list with the action put in place of its old self, or added
export function withAction(actions: Action[] | undefined, action: Action): Action[] {
  const list = actions ?? [];
  return action.id && list.some((a) => a.id === action.id) ? list.map((a) => (a.id === action.id ? action : a)) : [...list, action];
}

export const withoutAction = (actions: Action[] | undefined, id: string): Action[] => (actions ?? []).filter((a) => a.id !== id);

// actionProblem is why an action cannot be saved, "" when it can
export function actionProblem(action: Pick<Action, "name" | "command" | "folder">): string {
  if (!action.name.trim()) return t("Name the action.");
  if (!action.command.trim()) return t("Give the command it runs.");
  if (/[\r\n]/.test(action.command)) return t("One line: join commands with && or put them in a script.");
  const folder = (action.folder ?? "").trim();
  if (folder.startsWith("/") || folder.startsWith("~") || folder.split("/").includes("..") || /[\\:]/.test(folder)) return t("A folder inside the checkout, like web.");
  return "";
}

// whenWords is the "When to run" choice in words
export function whenWords(when: SetupWhen | undefined): string {
  return when === "ask" ? t("Ask every time") : when === "skip" ? t("Skip by default") : t("Run by default");
}

// setupLine is the setup the dialog shows on one line: the team's then the local one's
export function setupLine(team: string | undefined, local: string | undefined): string {
  return [team, local]
    .filter((s): s is string => !!s?.trim())
    .flatMap((s) => s.split("\n"))
    .map((s) => s.trim())
    .filter(Boolean)
    .join(" · ");
}

// sameSettings says whether an edit changed nothing
export const sameSettings = (a: Settings, b: Settings): boolean => JSON.stringify(normal(a)) === JSON.stringify(normal(b));

function normal(s: Settings): Settings {
  const out: Settings = {};
  for (const [k, v] of Object.entries(s)) if (v !== "" && v !== false && v !== undefined && !(Array.isArray(v) && v.length === 0)) (out as Record<string, unknown>)[k] = v;
  return out;
}
