// What a workspace's tab says above the pane: the agent it runs by its logo and name, or "terminal" for a shell, a
// name of the person's own over either; two tabs that would read the same are told apart by herdr's number.

import { t, tn } from "@/lib/i18n";
import { VENDOR_NAME } from "@/ui/brand";
import type { Tab } from "./api";

export type TabFace = { label: string; agent: string };

// tabAgent is the agent a tab runs: its focused pane's, else the first agent pane's, "" for a tab of shells
export function tabAgent(tab: Tab): string {
  const focused = tab.panes.find((p) => p.focused && p.agent);
  return (focused ?? tab.panes.find((p) => p.agent))?.agent ?? "";
}

// tabFaces names every tab of a workspace, by tab id; masked leaves out the names the person gave (screen-sharing mode)
export function tabFaces(tabs: Tab[], masked = false): Map<string, TabFace> {
  const faces = new Map<string, TabFace>();
  for (const tab of tabs) {
    const agent = tabAgent(tab);
    const label = tab.named && !masked ? tab.label : agent ? (VENDOR_NAME[agent] ?? agent) : t("terminal");
    faces.set(tab.id, { label, agent });
  }
  const seen = new Map<string, number>();
  for (const face of faces.values()) seen.set(face.label, (seen.get(face.label) ?? 0) + 1);
  for (const tab of tabs) {
    const face = faces.get(tab.id)!;
    if ((!tab.named || masked) && seen.get(face.label)! > 1) faces.set(tab.id, { ...face, label: `${face.label} ${tab.number}` });
  }
  return faces;
}

// closeTabAsk is the confirm before a tab closes: the tab named as the strip names it (no name of the person's own while
// the screen is shared), or, for the last one, the workspace that closes with it
export function closeTabAsk(tabs: Tab[], tab: Tab, masked: boolean, workspace: string): string {
  if (tabs.length === 1) return t("Close the last tab? Workspace “{workspace}” closes with it.", { workspace });
  const name = tabFaces(tabs, masked).get(tab.id)?.label ?? String(tab.number);
  return tn(tab.panes.length, "Close tab “{tab}”? Its {n} pane and agent stop.", "Close tab “{tab}”? Its {n} panes and agents stop.", { tab: name });
}
