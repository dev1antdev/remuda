// Scenes: the morning set-ups and the evening tidies, kept on this remuda and replaced whole; a run read back while it goes.

import { getJSON, sendJSON } from "@/lib/http";
import { useQuery } from "@/lib/query";
import { createStore } from "@/lib/store";

export type SceneKind = "start" | "tidy";

// LayoutNode is a pane or a split of a captured layout; a pane's dir is relative to the step's folder, or absolute outside it
export type LayoutNode = {
  type: "pane" | "split";
  dir?: string;
  label?: string;
  command?: string[];
  direction?: "right" | "down";
  ratio?: number;
  first?: LayoutNode;
  second?: LayoutNode;
};

// Layout is a tab's panes as a step lays them out again; agent is the pane the agent starts in, counted left to right, top to bottom
export type Layout = { root: LayoutNode; agent: number };

export type Step = {
  machine: string;
  dir: string;
  agent?: string; // an agent kind, "shell" for a plain shell; absent with a recipe
  recipe?: string; // a recipe's id: its agent, model, arguments and prompt, read when the step runs
  model?: string;
  args?: string;
  prompt?: string;
  tab?: string; // the tab's name
  layout?: Layout;
};

export type Tidy = {
  shells: boolean; // close the shells at their prompt
  idle: boolean; // close agents idle for idle_hours or more
  idle_hours?: number;
  compact: boolean; // /compact to idle Claude Codes past compact_at
  compact_at?: number; // percent of the context window
  worktrees: boolean; // remove the clean leftover worktrees
  machines?: string[]; // the machines it tidies, none for every one
};

export type Scene = { id?: string; name: string; kind: SceneKind; steps?: Step[]; tidy?: Tidy };

export type ActionKind = "compact" | "close-agent" | "close-shell" | "worktree";

// Action is one thing a tidy would do
export type Action = {
  id: string;
  kind: ActionKind;
  machine: string;
  pane?: string;
  workspace?: string;
  tab?: string;
  agent?: string;
  title?: string;
  since?: number; // unix ms the agent last worked
  context?: number; // percent of the window
  closes?: "tab" | "workspace"; // what goes with the pane, its last
  path?: string;
  branch?: string;
  kb?: number;
};

export type Unread = { machine: string; label: string; why: string };
export type Plan = { actions: Action[]; unread: Unread[] };

export type ItemState = "waiting" | "running" | "started" | "done" | "refused" | "away" | "skipped";
export type Item = { index: number; step?: Step; action?: Action; state: ItemState; note?: string; pane?: string; made_workspace?: boolean };
export type Run = { id: string; scene: string; name: string; kind: SceneKind; at: string; done: boolean; items: Item[] };

export type Captured = { layout: Layout; dir: string; agent: string };

export const fetchScenes = (): Promise<Scene[]> => getJSON("/api/scenes");
export const saveScenes = (list: Scene[]): Promise<Scene[]> => sendJSON<Scene[]>("PUT", "/api/scenes", list).then((saved) => (scenesChanged(), saved));
export const captureLayout = (machine: string, workspace: string): Promise<Captured> => sendJSON("POST", "/api/scenes/capture", { machine, workspace });
export const fetchPlan = (id: string): Promise<Plan> => getJSON(`/api/scenes/${encodeURIComponent(id)}/plan`);
export const runScene = (id: string, picked: { steps?: number[]; actions?: string[] }): Promise<Run> => sendJSON("POST", `/api/scenes/${encodeURIComponent(id)}/run`, picked);
export const fetchRun = (id: string): Promise<Run> => getJSON(`/api/scene-runs/${encodeURIComponent(id)}`);

// saved counts the saves, so every screen listing the scenes — the palette, the sidebar's + menu — reads them again
const saved = createStore({ n: 0 });
const scenesChanged = () => saved.set((s) => ({ n: s.n + 1 }));

// useScenes is the saved scenes for a menu or the palette, read once and again after every save; none for a viewer
export function useScenes(enabled: boolean): Scene[] {
  const n = saved.use((s) => s.n);
  return useQuery(fetchScenes, [n], { enabled, keep: true }).data ?? [];
}
