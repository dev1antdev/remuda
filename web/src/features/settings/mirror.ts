// Inside herdr's words: how a toast went on a machine, the tone of a test's answers, and the quiet hours kept as
// they were set.

import type { Quiet } from "@/features/alerts/push";
import { t } from "@/lib/i18n";
import type { MirrorSettings, ToastOutcome, Yield } from "./mirrorApi";

export type OutcomeTone = "success" | "warn" | "error";

// outcomeWords is one machine's toast in a line: shown, or why not in herdr's words made plain.
export function outcomeWords(outcome: ToastOutcome): string {
  const machine = outcome.label || outcome.machine;
  if (outcome.error) return t("{machine}: herdr was not reached — {error}", { machine, error: outcome.error });
  if (outcome.shown) return t("{machine}: shown", { machine });
  switch (outcome.reason) {
    case "disabled":
      return t("{machine}: herdr's toasts are off there; set ui.toast.delivery in its config.toml", { machine });
    case "no_foreground_client":
      return t("{machine}: nobody has herdr open there", { machine });
    case "rate_limited":
      return t("{machine}: herdr held it back, too many toasts at once", { machine });
    case "busy":
      return t("{machine}: another toast was still up there", { machine });
  }
  return t("{machine}: not shown ({reason})", { machine, reason: outcome.reason || "?" });
}

// elsewhereWords is a machine whose herdr another remuda writes into: the cockpit reaching it, so this one stays out.
export function elsewhereWords(where: Yield): string {
  const machine = where.label || where.machine;
  if (!where.writer) return t("On {machine} another remuda writes into herdr and pops its toasts, so this one stays out there.", { machine });
  return t("On {machine} the remuda at {writer} writes into herdr and pops its toasts, so this one stays out there.", { machine, writer: where.writer });
}

export function outcomeTone(outcome: ToastOutcome): OutcomeTone {
  if (outcome.error) return "error";
  return outcome.shown ? "success" : "warn";
}

// outcomesTone is the gravest of a test's answers: one machine out of reach outweighs the rest.
export function outcomesTone(outcomes: ToastOutcome[]): OutcomeTone {
  const tones = outcomes.map(outcomeTone);
  if (tones.includes("error")) return "error";
  if (tones.includes("warn")) return "warn";
  return "success";
}

// withQuiet is the settings with the quiet hours on (the window, in the zone given) or off.
export function withQuiet(settings: MirrorSettings, on: boolean, window: Omit<Quiet, "tz">, zone: string): MirrorSettings {
  const { quiet: _, ...rest } = settings;
  return on ? { ...rest, quiet: { ...window, tz: zone } } : rest;
}

// quietZone is the zone the quiet hours keep: the one they were set in, else this device's.
export const quietZone = (settings: MirrorSettings, deviceZone: string): string => settings.quiet?.tz || deviceZone;
