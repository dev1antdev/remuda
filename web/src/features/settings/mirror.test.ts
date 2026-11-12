import { afterEach, describe, expect, it } from "vitest";
import { setLang } from "@/lib/i18n";
import { outcomesTone, outcomeTone, outcomeWords, quietZone, withQuiet } from "./mirror";
import type { ToastOutcome } from "./mirrorApi";

afterEach(() => setLang("en"));

const at = "2026-11-12T10:00:00Z";
const outcome = (o: Partial<ToastOutcome>): ToastOutcome => ({ machine: "gog-id", label: "GOG", at, title: "remuda", shown: false, ...o });

describe("Inside herdr's toasts", () => {
  it("says how a toast went on a machine", () => {
    expect(outcomeWords(outcome({ shown: true, reason: "shown" }))).toBe("GOG: shown");
    expect(outcomeWords(outcome({ reason: "disabled" }))).toBe("GOG: herdr's toasts are off there; set ui.toast.delivery in its config.toml");
    expect(outcomeWords(outcome({ reason: "no_foreground_client" }))).toBe("GOG: nobody has herdr open there");
    expect(outcomeWords(outcome({ reason: "rate_limited" }))).toBe("GOG: herdr held it back, too many toasts at once");
    expect(outcomeWords(outcome({ reason: "busy" }))).toBe("GOG: another toast was still up there");
    expect(outcomeWords(outcome({ reason: "something new" }))).toBe("GOG: not shown (something new)");
    expect(outcomeWords(outcome({ label: "", error: "the machine is not connected" }))).toBe("gog-id: herdr was not reached — the machine is not connected");
    setLang("ru");
    expect(outcomeWords(outcome({ shown: true }))).toBe("GOG: показано");
  });

  it("weighs a test's answers by the gravest", () => {
    expect(outcomeTone(outcome({ shown: true }))).toBe("success");
    expect(outcomeTone(outcome({ reason: "disabled" }))).toBe("warn");
    expect(outcomesTone([outcome({ shown: true }), outcome({ reason: "busy" })])).toBe("warn");
    expect(outcomesTone([outcome({ shown: true }), outcome({ error: "x" }), outcome({ reason: "busy" })])).toBe("error");
    expect(outcomesTone([outcome({ shown: true })])).toBe("success");
  });

  it("keeps the quiet hours in the zone they were set in", () => {
    const settings = { figures: true, toasts: true };
    const on = withQuiet(settings, true, { from: "22:00", to: "07:00" }, "Europe/Kyiv");
    expect(on).toEqual({ figures: true, toasts: true, quiet: { from: "22:00", to: "07:00", tz: "Europe/Kyiv" } });
    expect(withQuiet(on, false, { from: "22:00", to: "07:00" }, "Europe/Kyiv")).toEqual(settings);
    expect(quietZone(on, "America/New_York")).toBe("Europe/Kyiv");
    expect(quietZone(settings, "America/New_York")).toBe("America/New_York");
  });
});
