// Budgets as the page reads them: which limits apply to a pane, how far along each is, the tone that says so,
// and the words for a row, a tooltip or a push. Pure, so the pill and the panel agree.

import { basename } from "@/lib/text";
import type { BudgetStatus, Period } from "./api";
import { usd } from "./format";
import { t, tn } from "@/lib/i18n";

export type BudgetTone = "" | "dim" | "amber" | "red";

export const PERIODS: { value: Period; label: string }[] = [
  { value: "day", label: t("A day") },
  { value: "week", label: t("A week (from Monday)") },
  { value: "month", label: t("A month") },
];

// ratio is how far along a budget is: spent over its limit, 0 while the spend is not known yet
export const ratio = (b: BudgetStatus): number => (b.collected && b.limit > 0 ? b.spent / b.limit : 0);

// percentOf is a ratio as the whole percent the words show, a hair of rounding forgiven so 2.4 of 3 reads 80
const percentOf = (r: number): number => Math.floor(100 * r + 1e-9);

// budgetTone: nothing under half, dim from half, amber from 80 %, red at the limit — by the percent the words say
export function budgetTone(r: number): BudgetTone {
  const pct = percentOf(r);
  if (pct >= 100) return "red";
  if (pct >= 80) return "amber";
  if (pct >= 50) return "dim";
  return "";
}

const GRAVITY: Record<BudgetTone, number> = { "": 0, dim: 1, amber: 2, red: 3 };

// graver is the more pressing of two tones
export const graver = (a: BudgetTone, b: BudgetTone): BudgetTone => (GRAVITY[a] >= GRAVITY[b] ? a : b);

// toneClass is the pill's class for a tone: the context pill's own amber and red, nothing for the rest
export const toneClass = (tone: BudgetTone): string => (tone === "red" ? " is-red" : tone === "amber" ? " is-amber" : "");

// budgetsFor lists the budgets that cover a pane: every machine, its machine, its machine and folder
export function budgetsFor(budgets: BudgetStatus[] | undefined, machineId: string, dir: string): BudgetStatus[] {
  return (budgets ?? []).filter((b) => !b.machine || (b.machine === machineId && (!b.dir || b.dir === dir)));
}

// tightest is the budget furthest along, the one whose tone the pill wears
export function tightest(list: BudgetStatus[]): BudgetStatus | null {
  let best: BudgetStatus | null = null;
  for (const b of list) if (!best || ratio(b) > ratio(best)) best = b;
  return best;
}

// periodWords: today, this week, this month
export const periodWords = (p: Period): string => (p === "week" ? t("this week") : p === "month" ? t("this month") : t("today"));

// budgetWords: "$16.40 of $20.0 this week · 82 %", or what is known while the spend is not read yet; a spend many
// times the limit says so instead of a number nobody reads
export function budgetWords(b: BudgetStatus): string {
  if (!b.collected) return t("{limit} {period} · reading…", { limit: usd(b.limit), period: periodWords(b.period) });
  const pct = percentOf(ratio(b));
  return t("{spent} of {limit} {period} · {pct}", { spent: usd(b.spent), limit: usd(b.limit), period: periodWords(b.period), pct: pct > 999 ? t("far past the limit") : `${pct} %` });
}

// missingWords says which machines a reading left out, "" when none: "GOG not counted: no report from it"
export function missingWords(b: BudgetStatus, machineLabel: (id: string) => string): string {
  if (!b.missing?.length) return "";
  return tn(b.missing.length, "{machines} not counted: no report from it", "{machines} not counted: no report from them", { machines: b.missing.map(machineLabel).join(", ") });
}

// scopeWords: "everything", a machine's name, or "<machine> · <folder>"
export function scopeWords(b: BudgetStatus, machineLabel: (id: string) => string): string {
  if (!b.machine) return t("everything");
  if (!b.dir) return machineLabel(b.machine);
  return `${machineLabel(b.machine)} · ${basename(b.dir)}`;
}
