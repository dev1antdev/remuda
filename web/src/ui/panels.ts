// The sizes of the panels a person can drag: each a pref per device, clamped to its range, stepped by the keyboard — a
// width for a panel beside the page, a height for the floating terminal's drawer, whose top edge moves.

export type Edge = "left" | "right" | "top"; // the panel's edge that moves: a sidebar's right one, a rail's left one, a drawer's top
export type PanelSpec = { key: string; cssVar: string; width: number; min: number; max: number };

export const PANELS = {
  sidebar: { key: "panel.sidebar", cssVar: "--sidebar-w", width: 268, min: 200, max: 520 },
  tasksSidebar: { key: "panel.tasks.sidebar", cssVar: "--tasks-sidebar-w", width: 268, min: 180, max: 480 },
  rail: { key: "panel.rail", cssVar: "--rail-w", width: 300, min: 220, max: 600 },
  queue: { key: "panel.queue", cssVar: "--queue-w", width: 300, min: 220, max: 560 },
  taskDetail: { key: "panel.tasks.detail", cssVar: "--task-detail-w", width: 380, min: 300, max: 720 },
  split: { key: "panel.split", cssVar: "--split-w", width: 520, min: 320, max: 1200 },
  float: { key: "panel.float", cssVar: "--float-h", width: 320, min: 160, max: 900 }, // a height: the drawer's top edge
} satisfies Record<string, PanelSpec>;

export const STEP = 16; // one arrow key; Shift makes it four

// clampWidth keeps a width inside the panel's range, whole pixels; anything that is not a number is the usual width
export function clampWidth(spec: PanelSpec, width: number): number {
  if (!Number.isFinite(width)) return spec.width;
  return Math.min(spec.max, Math.max(spec.min, Math.round(width)));
}

// dragWidth is the size once the edge moved d pixels right (or down, for a top edge): a right edge grows the panel, a
// left one shrinks it, and a top edge moved down shrinks the drawer under it
export function dragWidth(spec: PanelSpec, start: number, d: number, edge: Edge): number {
  return clampWidth(spec, edge === "right" ? start + d : start - d);
}

// keyWidth is the size a key asks of the focused edge — the arrows along it move it, Home and End go to the ends of
// the range, Enter brings the usual size back — and null for a key that is not the edge's
export function keyWidth(spec: PanelSpec, width: number, key: string, shift: boolean, edge: Edge): number | null {
  const step = shift ? STEP * 4 : STEP;
  const vertical = edge === "top";
  switch (key) {
    case "ArrowRight":
      return vertical ? null : dragWidth(spec, width, step, edge);
    case "ArrowLeft":
      return vertical ? null : dragWidth(spec, width, -step, edge);
    case "ArrowUp":
      return vertical ? dragWidth(spec, width, -step, edge) : null;
    case "ArrowDown":
      return vertical ? dragWidth(spec, width, step, edge) : null;
    case "Home":
      return spec.min;
    case "End":
      return spec.max;
    case "Enter":
      return spec.width;
    default:
      return null;
  }
}
