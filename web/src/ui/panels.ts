// The widths of the panels a person can drag: each a pref per device, clamped to its range, stepped by the keyboard.

export type Edge = "left" | "right"; // the panel's edge that moves: a sidebar's right one, a rail's left one
export type PanelSpec = { key: string; cssVar: string; width: number; min: number; max: number };

export const PANELS = {
  sidebar: { key: "panel.sidebar", cssVar: "--sidebar-w", width: 268, min: 200, max: 520 },
  tasksSidebar: { key: "panel.tasks.sidebar", cssVar: "--tasks-sidebar-w", width: 268, min: 180, max: 480 },
  rail: { key: "panel.rail", cssVar: "--rail-w", width: 300, min: 220, max: 600 },
  queue: { key: "panel.queue", cssVar: "--queue-w", width: 300, min: 220, max: 560 },
  taskDetail: { key: "panel.tasks.detail", cssVar: "--task-detail-w", width: 380, min: 300, max: 720 },
  split: { key: "panel.split", cssVar: "--split-w", width: 520, min: 320, max: 1200 },
} satisfies Record<string, PanelSpec>;

export const STEP = 16; // one arrow key; Shift makes it four

// clampWidth keeps a width inside the panel's range, whole pixels; anything that is not a number is the usual width
export function clampWidth(spec: PanelSpec, width: number): number {
  if (!Number.isFinite(width)) return spec.width;
  return Math.min(spec.max, Math.max(spec.min, Math.round(width)));
}

// dragWidth is the width once the edge moved dx pixels to the right: a right edge grows the panel, a left one shrinks it
export function dragWidth(spec: PanelSpec, start: number, dx: number, edge: Edge): number {
  return clampWidth(spec, edge === "right" ? start + dx : start - dx);
}

// keyWidth is the width a key asks of the focused edge — the arrows move it, Home and End go to the ends of the range,
// Enter brings the usual width back — and null for a key that is not the edge's
export function keyWidth(spec: PanelSpec, width: number, key: string, shift: boolean, edge: Edge): number | null {
  const step = shift ? STEP * 4 : STEP;
  switch (key) {
    case "ArrowRight":
      return dragWidth(spec, width, step, edge);
    case "ArrowLeft":
      return dragWidth(spec, width, -step, edge);
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
