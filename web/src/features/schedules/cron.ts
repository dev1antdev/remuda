// The presets behind a schedule's "When", and the cron each one writes.

export const NEVER = "0001-01-01T00:00:00Z"; // Go's zero time
export const DEFAULT_CRON = "0 9 * * 1-5";

export type Preset = "daily" | "weekdays" | "hourly" | "custom";

export const PRESETS: { id: Preset; label: string }[] = [
  { id: "daily", label: "Every day" },
  { id: "weekdays", label: "Weekdays" },
  { id: "hourly", label: "Every hour" },
  { id: "custom", label: "Cron…" },
];

// cronFor is the cron a preset means at hh:mm; custom leaves the cron as typed
export function cronFor(preset: Preset, hh: number, mm: number): string | null {
  switch (preset) {
    case "daily":
      return `${mm} ${hh} * * *`;
    case "weekdays":
      return `${mm} ${hh} * * 1-5`;
    case "hourly":
      return `${mm} * * * *`;
    default:
      return null;
  }
}

// parseCron is a cron a preset would have written → that preset, with its time
export function parseCron(cron: string): { preset: Preset; hh: number; mm: number } {
  const f = cron.trim().split(/\s+/);
  const mm = Number(f[0]);
  const hh = Number(f[1]);
  if (f.length === 5 && Number.isInteger(mm) && f[2] === "*" && f[3] === "*") {
    if (f[1] === "*") return { preset: "hourly", hh: 9, mm };
    if (Number.isInteger(hh) && f[4] === "*") return { preset: "daily", hh, mm };
    if (Number.isInteger(hh) && f[4] === "1-5") return { preset: "weekdays", hh, mm };
  }
  return { preset: "custom", hh: Number.isInteger(hh) ? hh : 9, mm: Number.isInteger(mm) ? mm : 0 };
}
