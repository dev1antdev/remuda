import { t } from "@/lib/i18n";
// The presets behind a schedule's "When", and the cron each one writes.

export const NEVER = "0001-01-01T00:00:00Z"; // Go's zero time
export const DEFAULT_CRON = "0 9 * * 1-5";

export type Preset = "daily" | "weekdays" | "every" | "hourly" | "every3h" | "custom";

export const PRESETS: { id: Preset; label: string }[] = [
  { id: "daily", label: t("Every day") },
  { id: "weekdays", label: t("Weekdays") },
  { id: "every", label: t("Every N minutes") },
  { id: "hourly", label: t("Every hour") },
  { id: "every3h", label: t("Every 3 hours") },
  { id: "custom", label: t("Cron…") },
];

// STEPS are the minutes the "Every N minutes" preset offers: each divides the hour, so every hour starts on its :00
export const STEPS = [5, 10, 15, 20, 30];

// When is what the form's "When" row holds: the preset, its time of day, and the "every N minutes" window — from the hour
// `from` up to the hour `until` (24 for midnight), on weekdays only or every day
export type When = { preset: Preset; hh: number; mm: number; step: number; from: number; until: number; weekdays: boolean };

const WINDOW = { step: 10, from: 9, until: 17, weekdays: true };

// minuteOnly says whether a preset asks for a minute alone, no hour
export const minuteOnly = (preset: Preset): boolean => preset === "hourly" || preset === "every3h";

// hourField is the cron's hour field for the hours from `from` up to `until`, the last run in the hour before `until`
function hourField(from: number, until: number): string {
  const last = Math.max(from, until - 1);
  if (from === 0 && last === 23) return "*";
  return last === from ? `${from}` : `${from}-${last}`;
}

// cronFor is the cron a preset means; custom leaves the cron as typed
export function cronFor(when: When): string | null {
  const { hh, mm } = when;
  switch (when.preset) {
    case "daily":
      return `${mm} ${hh} * * *`;
    case "weekdays":
      return `${mm} ${hh} * * 1-5`;
    case "every":
      return `*/${when.step} ${hourField(when.from, when.until)} * * ${when.weekdays ? "1-5" : "*"}`;
    case "hourly":
      return `${mm} * * * *`;
    case "every3h":
      return `${mm} */3 * * *`;
    default:
      return null;
  }
}

// hoursOf reads an hour field the "every N minutes" preset writes as its window, null for any other
function hoursOf(field: string): { from: number; until: number } | null {
  if (field === "*") return { from: 0, until: 24 };
  const m = /^(\d{1,2})(?:-(\d{1,2}))?$/.exec(field);
  if (!m) return null;
  const from = Number(m[1]);
  const last = m[2] === undefined ? from : Number(m[2]);
  return from <= last && last <= 23 ? { from, until: last + 1 } : null;
}

// parseCron is a cron a preset would have written → that preset, with its time and window
export function parseCron(cron: string): When {
  const f = cron.trim().split(/\s+/);
  const mm = Number(f[0]);
  const hh = Number(f[1]);
  const base = { ...WINDOW, hh: Number.isInteger(hh) ? hh : 9, mm: Number.isInteger(mm) ? mm : 0 };
  if (f.length !== 5 || f[2] !== "*" || f[3] !== "*") return { ...base, preset: "custom" };
  const step = /^\*\/(\d+)$/.exec(f[0]!);
  const window = hoursOf(f[1]!);
  if (step && STEPS.includes(Number(step[1])) && window && (f[4] === "*" || f[4] === "1-5")) {
    return { ...base, hh: 9, mm: 0, preset: "every", step: Number(step[1]), ...window, weekdays: f[4] === "1-5" };
  }
  if (Number.isInteger(mm)) {
    if (f[1] === "*" && f[4] === "*") return { ...base, hh: 9, preset: "hourly" };
    if (f[1] === "*/3" && f[4] === "*") return { ...base, hh: 9, preset: "every3h" };
    if (Number.isInteger(hh) && f[4] === "*") return { ...base, preset: "daily" };
    if (Number.isInteger(hh) && f[4] === "1-5") return { ...base, preset: "weekdays" };
  }
  return { ...base, preset: "custom" };
}
