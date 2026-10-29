// A page a viewer link signed in: it draws what the full page draws and hides every control that would type or change
// anything. The server refuses those calls anyway (403); this keeps the buttons from being there. The pure part.

// VIEWER_MODES are the modes a viewer has: the agents and the Wall; the tasks, the code and the review all write
const VIEWER_MODES = new Set(["herdr", "wall"]);

// VIEWER_PANELS are the panels a viewer has: the reports and the feed; the schedules are the person's own
const VIEWER_PANELS = new Set(["standup", "usage", "activity"]);

// viewerPlaces keeps the rail's places a viewer may open; the full list stands for everyone else
export function viewerPlaces<T extends { mode?: string; panel?: string | null }>(places: T[], viewOnly: boolean): T[] {
  if (!viewOnly) return places;
  return places.filter((place) => (place.mode ? VIEWER_MODES.has(place.mode) : place.panel ? VIEWER_PANELS.has(place.panel) : false));
}
