// A page a viewer link signed in: it draws what the full page draws and hides every control that would type or change
// anything. The server refuses those calls anyway (403); this keeps the buttons from being there. The pure part.

// VIEWER_MODES are the modes a viewer has: the agents and the Wall; the tasks, the code and the review all write
const VIEWER_MODES = new Set(["herdr", "wall"]);

// VIEWER_PANELS are the panels a viewer has: the reports, the feed and a server's vitals (its panel hides its buttons
// for a viewer); the schedules are the person's own
const VIEWER_PANELS = new Set(["standup", "usage", "activity", "server"]);

// viewerPlaces keeps the rail's places a viewer may open; the full list stands for everyone else
export function viewerPlaces<T extends { mode?: string; panel?: string | null }>(places: T[], viewOnly: boolean): T[] {
  if (!viewOnly) return places;
  return places.filter((place) => (place.mode ? VIEWER_MODES.has(place.mode) : place.panel ? VIEWER_PANELS.has(place.panel) : false));
}

// VIEWER_COMMANDS are the palette's commands a viewer keeps: moving about, what a lens shows, the reports, the keys; the
// look's rows and the screen-sharing mode go by their prefixes. Every other one types, starts, closes, renames or saves
// something on the server.
const VIEWER_COMMANDS = new Set(["next-needs", "mode:herdr", "mode:wall", "lens:chat", "lens:terminal", "lens:git", "find", "timeline", "export", "recap", "pins", "timelapse", "popout", "find-viewer", "timeline-viewer", "export-viewer", "recap-viewer", "wall-needs", "wall-idle", "wall-unfocus", "wall-focus", "wall-office", "wall-view", "wall-zoo", "full", "rail", "panels", "split", "activity", "usage", "scorecard", "standup", "listen-day", "budgets", "keys", "server"]);
const VIEWER_COMMAND_PREFIXES = ["theme-", "text-", "lang-", "open-lens-", "sharing"];

// viewerCommand says whether a viewer's palette lists the command with this id
export function viewerCommand(id: string): boolean {
  return VIEWER_COMMANDS.has(id) || VIEWER_COMMAND_PREFIXES.some((prefix) => id.startsWith(prefix));
}

// ViewerToast is as much of a toast as the rule needs: its id and its button
export type ViewerToast = { id: string; action?: unknown };

// viewerToasts is what a viewer's corner shows: no Resume of stopped chats and no install of a release (the server would
// refuse them), no dev server offered as a preview (the previews are the person's), no plan limit's offer to run held
// work on the other vendor (only the person may), and no Tailscale login, whose link would put this cockpit on the
// tailnet of whoever follows it
export function viewerToasts<T extends ViewerToast>(toasts: T[]): T[] {
  return toasts.filter((toast) => toast.id !== "tailscale" && !toast.id.startsWith("serves-") && !toast.id.startsWith("plan-offer-")).map((toast) => (toast.id === "release" || toast.id.startsWith("recover-") ? { ...toast, action: undefined } : toast));
}

// viewerMenu is the first group of a menu, up to its first separator: a pull request's Open, without the hand-offs to the agent
export function viewerMenu<T extends object>(items: T[]): T[] {
  const sep = items.findIndex((item) => "sep" in item);
  return sep < 0 ? items : items.slice(0, sep);
}
