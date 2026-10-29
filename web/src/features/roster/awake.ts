// The words for a machine's keep-awake switch: what the ⓘ says about the hold as it stands.

import { t, tn } from "@/lib/i18n";
import type { Awake } from "./api";

// awakeWords is the ⓘ's Awake row: held with agents working, held through the grace, failed, or on with nothing working.
export function awakeWords(a: Awake | undefined): string {
  if (!a?.on) return t("Off");
  if (a.held && a.working > 0) return tn(a.working, "Kept awake · {n} agent working", "Kept awake · {n} agents working");
  if (a.held) return t("Kept awake · nothing working, let go in a couple of minutes");
  if (a.error) return t("Could not hold: {error}", { error: a.error });
  if (a.working > 0) return tn(a.working, "Taking the hold · {n} agent working", "Taking the hold · {n} agents working");
  return t("On · nothing working now");
}

// awakeBad says whether the row needs the warning colour: the switch is on and the hold failed.
export const awakeBad = (a: Awake | undefined): boolean => !!a?.on && !a.held && !!a.error;
