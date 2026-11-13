// The project settings' and the run button's calls; a save wakes every screen that shows a project's settings.

import { getJSON, sendJSON } from "@/lib/http";
import { createStore } from "@/lib/store";
import type { Project, Settings } from "./project";

const q = encodeURIComponent;

// saved counts the saves made from this page: the run button and the dialogs read a project again on each
export const saved = createStore({ n: 0 });

// fetchProject reads a project's settings; quick leaves out the read of the repository on its machine (its team's
// script and default base), which the run button never shows
export const fetchProject = (machine: string, root: string, quick = false): Promise<Project> =>
  getJSON(`/api/repos/settings?machine=${q(machine)}&root=${q(root)}${quick ? "&quick=1" : ""}`);

export async function saveProject(machine: string, root: string, settings: Settings): Promise<Settings> {
  const answer = await sendJSON<{ settings: Settings }>("PUT", "/api/repos/settings", { machine, root, settings });
  saved.set((s) => ({ n: s.n + 1 }));
  return answer.settings;
}

// runAction starts a saved action in a shell herdr makes for it; the answer is its pane
export const runAction = (req: { machine: string; workspace: string; pane: string; root: string; action: string }): Promise<{ pane: string; place: string }> => sendJSON("POST", "/api/run", req);
