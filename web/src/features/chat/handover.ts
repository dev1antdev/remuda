import { t } from "@/lib/i18n";
// Handing a conversation to another agent: the words of the dialog and of the toast, and the choices it offers.

export const DEFAULT_TURNS = 3;
export const MAX_TURNS = 10;

// clampTurns keeps the number of exchanges carried in full within what the server takes
export function clampTurns(n: number): number {
  if (!Number.isFinite(n)) return DEFAULT_TURNS;
  return Math.min(MAX_TURNS, Math.max(0, Math.round(n)));
}

// turnOptions is the Select over how much goes over in full: the recap alone, or the last N exchanges
export function turnOptions(): { value: number; label: string }[] {
  const out = [{ value: 0, label: t("The recap only") }];
  for (let n = 1; n <= MAX_TURNS; n++) out.push({ value: n, label: n === 1 ? t("The last exchange") : t("The last {n} exchanges", { n }) });
  return out;
}

// handoverWords says what the new agent gets as its first prompt
export function handoverWords(turns: number): string {
  if (turns <= 0) return "the recap of this conversation";
  if (turns === 1) return "the recap and the last exchange in full";
  return t("the recap and the last {n} exchanges in full", { n: turns });
}

// nextAgent is the kind the dialog offers first: another than the one in the pane, else that one, else none
export function nextAgent(kinds: string[], current: string): string {
  return kinds.find((k) => k !== current) ?? kinds[0] ?? "";
}

// handedOverWords is the toast once the new tab is up: who took over and how much they got
export function handedOverWords(label: string, size: number): string {
  const kb = Math.max(1, Math.round(size / 1024));
  return t("Handed over to {agent}: {kb} KB sent as its first prompt", { agent: label, kb });
}
