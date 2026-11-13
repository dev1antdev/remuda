// What the lens compares, in the head's words as gitview names it — the uncommitted work, or the branch's whole work
// against the branch it was cut from — what the remote does not have yet, and the agent whose work a write would land on.

import { t, tn } from "@/lib/i18n";
import { underFolder } from "@/lib/pathname";
import type { Machine } from "@/features/roster/api";
import type { BaseView, Outgoing } from "./api";

export type Scope = "uncommitted" | "base";

// outgoingTitle is the group's name: what is committed and where it is not yet
export const outgoingTitle = (outgoing: Outgoing): string => t("Committed — not on {ref}", { ref: outgoing.against });

// pullWords is how far the upstream is ahead, as of the last fetch; "" when it is not, or there is no upstream
export const pullWords = (outgoing: Outgoing | undefined): string => (outgoing?.upstream && outgoing.behind ? tn(outgoing.behind, "{n} to pull", "{n} to pull") : "");

// baseWords is the head's line in the base scope: against what, and how far the branch went
export function baseWords(view: BaseView | null): string {
  if (!view) return t("Finding the branch it was cut from…");
  if (!view.base) return t("No branch to compare with");
  const ahead = tn(view.ahead, "{n} commit ahead", "{n} commits ahead");
  return `${t("vs {base}", { base: view.base })} · ${ahead}${view.merge_base ? ` · ${t("since {sha}", { sha: view.merge_base.slice(0, 7) })}` : ""}`;
}

// AUTO_BASE is the pick that leaves the base to remuda
export const AUTO_BASE = "";

// baseChoices is the branches a base may be pinned to: remuda's own find first, then the newest branches
export function baseChoices(view: BaseView | null): { value: string; label: string; hint?: string }[] {
  const detected = view?.detected ?? "";
  const choices: { value: string; label: string; hint?: string }[] = [{ value: AUTO_BASE, label: detected ? t("Detected: {base}", { base: detected }) : t("Detected"), hint: t("the branch this one left most recently") }];
  for (const ref of view?.refs ?? []) if (ref) choices.push({ value: ref, label: ref });
  return choices;
}

// isSubfolder says the lens's folder lies inside the repository rather than at its top; git names the top by its real
// path (a Mac's /private/var for /var), so a top that ends with the folder is the folder itself
export function isSubfolder(repo: string | undefined, dir: string): boolean {
  const top = (repo ?? "").replace(/[\\/]+$/, "");
  const folder = dir.replace(/[\\/]+$/, "");
  return !!top && top !== folder && !top.endsWith(folder);
}

// busyAgent is the agent at work, or asking, in the checkout as the roster has it: a pane whose workspace folder is
// inside the checkout (or holds the lens's folder); "" when none, so the server's own word then stands
export function busyAgent(machine: Machine | undefined, repo: string | undefined, dir: string, agentLabel: (kind: string) => string): string {
  if (!machine || !repo) return "";
  const inside = (folder: string, root: string) => folder === root.replace(/[\\/]+$/, "") || underFolder(folder, root) !== null;
  for (const session of machine.sessions) {
    const folder = (session.dir ?? "").replace(/[\\/]+$/, "");
    if (!folder || !(inside(folder, repo) || inside(folder, dir))) continue;
    for (const tab of session.tabs)
      for (const pane of tab.panes) {
        const state = pane.radar?.state;
        if (pane.agent && (state === "working" || state === "blocked")) return agentLabel(pane.agent);
      }
  }
  return "";
}
