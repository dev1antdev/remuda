// Budgets as the page reads them: which limits apply to a pane, how far along each is, the tone that says so,
// and the words for a row, a tooltip or a push. Pure, so the pill and the panel agree.

import { basename } from "@/lib/text";
import type { BudgetStatus, Period } from "./api";
import { usd } from "./format";
import { t } from "@/lib/i18n";

export type BudgetTone = "" | "dim" | "amber" | "red";

export const PERIODS: { value: Period; label: string }[] = [
  { value: "day", label: t("A day") },
  { value: "week", label: "A week (from Monday)" },
  { value: "month", label: t("A month") },
];

// ratio is how far along a budget is: spent over its limit, 0 while the spend is not known yet
export const ratio = (b: BudgetStatus): number => (b.collected && b.limit > 0 ? b.spent / b.limit : 0);

// budgetTone: nothing under half, dim from half, amber from 80 %, red at the limit
export function budgetTone(r: number): BudgetTone {
  if (r >= 1) return "red";
  if (r >= 0.8) return "amber";
  if (r >= 0.5) return "dim";
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
export const periodWords = (p: Period): string => (p === "week" ? "this week" : p === "month" ? "this month" : "today");

// budgetWords: "$16.40 of $20.0 this week · 82 %", or what is known while the spend is not read yet; a spend many
// times the limit says so instead of a number nobody reads
export function budgetWords(b: BudgetStatus): string {
  if (!b.collected) return `${usd(b.limit)} ${periodWords(b.period)} · reading…`;
  const pct = Math.floor(100 * ratio(b) + 1e-9);
  return `${usd(b.spent)} of ${usd(b.limit)} ${periodWords(b.period)} · ${pct > 999 ? "far past the limit" : `${pct} %`}`;
}

// missingWords says which machines a reading left out, "" when none: "GOG not counted: no report from it"
export function missingWords(b: BudgetStatus, machineLabel: (id: string) => string): string {
  if (!b.missing?.length) return "";
  return `${b.missing.map(machineLabel).join(", ")} not counted: no report from ${b.missing.length === 1 ? "it" : "them"}`;
}

// scopeWords: "everything", a machine's name, or "<machine> · <folder>"
export function scopeWords(b: BudgetStatus, machineLabel: (id: string) => string): string {
  if (!b.machine) return "everything";
  if (!b.dir) return machineLabel(b.machine);
  return `${machineLabel(b.machine)} · ${basename(b.dir)}`;
}
