// The words of the Backups section: a copy's size, the last run, the next nightly one, the choices of how many stay.

import { locale, t } from "@/lib/i18n";
import { ago } from "@/lib/time";
import type { BackupResult } from "./backupApi";

export const KEEP_CHOICES = [3, 7, 14, 30, 60, 90];

// bytesWords is a copy's size: "640 KB", "1.2 MB"
export function bytesWords(size: number): string {
  if (size < 1024 * 1024) return t("{n} KB", { n: Math.max(1, Math.round(size / 1024)) });
  return t("{n} MB", { n: (size / 1024 / 1024).toFixed(1) });
}

// lastWords is what the last run came to: where it went, how big, or why it did not
export function lastWords(result: BackupResult, now = Date.now()): string {
  const when = ago(result.at, now);
  const where = result.label || result.machine;
  if (result.error) return t("Failed {when}, to {machine}: {error}", { when, machine: where, error: result.error });
  return t("Taken {when} to {machine} · {size} · {files} files", { when, machine: where, size: bytesWords(result.size ?? 0), files: result.files ?? 0 });
}

const clock = (d: Date) => d.toLocaleTimeString(locale() ?? [], { hour: "2-digit", minute: "2-digit" });

// nextWords is when the nightly run comes: today or tomorrow at its time
export function nextWords(next: string, now = new Date()): string {
  const at = new Date(next);
  const tomorrow = new Date(now);
  tomorrow.setDate(now.getDate() + 1);
  if (at.toDateString() === now.toDateString()) return t("next today at {time}", { time: clock(at) });
  if (at.toDateString() === tomorrow.toDateString()) return t("next tomorrow at {time}", { time: clock(at) });
  return t("next on {date}", { date: at.toLocaleDateString(locale() ?? [], { day: "numeric", month: "short" }) + " " + clock(at) });
}

// validTime says whether the field holds a time the server takes: HH:MM
export const validTime = (value: string): boolean => /^([01]\d|2[0-3]):[0-5]\d$/.test(value);
