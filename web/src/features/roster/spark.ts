// A sparkline's geometry: a day's samples as a path, a threshold as a height, the range a series is drawn in,
// and the words of its title. The ⓘ draws load and disk free from the vitals' history with it.

export type Threshold = { value: number; tone: "amber" | "red" };

const round = (n: number) => Math.round(n * 100) / 100;
const clamp = (v: number, min: number, max: number) => Math.min(max, Math.max(min, v));

// yOf maps a value into the chart, min at the bottom and max at the top.
function yOf(value: number, min: number, max: number, height: number): number {
  const span = max - min || 1;
  return round(height - ((clamp(value, min, max) - min) / span) * height);
}

// sparkPath is the line through the values, left to right across the width; one value is a flat line, none is nothing.
export function sparkPath(values: number[], width: number, height: number, min: number, max: number): string {
  if (values.length === 0) return "";
  if (values.length === 1) {
    const y = yOf(values[0]!, min, max, height);
    return `M0 ${y} L${width} ${y}`;
  }
  const step = width / (values.length - 1);
  return values.map((v, i) => `${i === 0 ? "M" : "L"}${round(i * step)} ${yOf(v, min, max, height)}`).join(" ");
}

// areaPath closes the line down to the chart's floor, for a light fill under it.
export function areaPath(line: string, width: number, height: number): string {
  return line ? `${line} L${width} ${height} L0 ${height} Z` : "";
}

// thresholdY is where a threshold sits in the chart, null when it lies outside the range drawn.
export function thresholdY(value: number, min: number, max: number, height: number): number | null {
  if (value < min || value > max) return null;
  return yOf(value, min, max, height);
}

// rangeOf is the range a series is drawn in: from the floor up to the ceiling, or past it when a value goes higher.
export function rangeOf(values: number[], floor: number, ceiling: number): { min: number; max: number } {
  const top = values.reduce((m, v) => Math.max(m, v), 0);
  return { min: floor, max: Math.max(ceiling, round(top * 1.05)) || 1 };
}

// loadThresholds: a load of one per core is amber, two per core red; nothing when the core count is unknown.
export function loadThresholds(cores: number | undefined): Threshold[] {
  if (!cores || cores <= 0) return [];
  return [
    { value: cores, tone: "amber" },
    { value: 2 * cores, tone: "red" },
  ];
}

// DISK_THRESHOLDS are the ⓘ's own: 10 % free is a concern, 5 % a bad one.
export const DISK_THRESHOLDS: Threshold[] = [
  { value: 10, tone: "amber" },
  { value: 5, tone: "red" },
];

// spanWords says how long the samples cover: "last 22 h", "last 30 min".
export function spanWords(from: string, to: string): string {
  const ms = new Date(to).getTime() - new Date(from).getTime();
  if (!Number.isFinite(ms) || ms < 0) return "";
  const minutes = Math.round(ms / 60000);
  if (minutes < 60) return `last ${minutes} min`;
  return `last ${Math.round(minutes / 60)} h`;
}

// sparkTitle is the chart's tooltip: the least, the most and the latest value, and the span.
export function sparkTitle(label: string, values: number[], fmt: (v: number) => string, span: string): string {
  if (values.length === 0) return label;
  const min = values.reduce((m, v) => Math.min(m, v), Infinity);
  const max = values.reduce((m, v) => Math.max(m, v), -Infinity);
  return `${label}: min ${fmt(min)}, max ${fmt(max)}, now ${fmt(values[values.length - 1]!)}${span ? ` · ${span}` : ""}`;
}
