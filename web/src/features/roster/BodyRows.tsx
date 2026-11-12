// The body's rows of a machine's vitals — memory, temperature, the CPU's clock, a Raspberry Pi's throttling, the
// battery — with the day's sparklines, inside the ⓘ's list and the Server place's. A row the machine does not report is
// not drawn.

import type { Health } from "./api";
import { batteryLow, batteryWords, clockWords, memUsedPct, memWords, MEM_THRESHOLDS, TEMP_THRESHOLDS, tempWords, throttleWords } from "./body";
import { Sparkline } from "./Sparkline";
import { sparkTitle } from "./spark";
import { t } from "@/lib/i18n";

export function BodyRows({ health: h, span }: { health: Health; span: string }) {
  const history = h.history && h.history.length >= 2 ? h.history : null;
  const mems = history ? history.filter((s) => s.mem).map((s) => s.mem!) : [];
  const temps = history ? history.filter((s) => s.temp).map((s) => s.temp!) : [];
  const mem = memWords(h);
  const temp = tempWords(h);
  const clock = clockWords(h);
  const used = memUsedPct(h);
  return (
    <>
      {mem && (
        <>
          <dt>{t("Memory")}</dt>
          <dd className={used !== null && used >= 90 ? "health-body-warn" : undefined}>
            {mem}
            {mems.length >= 2 && <Sparkline values={mems} min={0} max={100} color="var(--ctp-mauve)" thresholds={MEM_THRESHOLDS} title={sparkTitle(t("Memory in use"), mems, (v) => `${v.toFixed(0)}%`, span)} />}
          </dd>
        </>
      )}
      {temp && (
        <>
          <dt>{t("Temperature")}</dt>
          <dd>
            {temp}
            {temps.length >= 2 && <Sparkline values={temps} min={30} max={Math.max(90, ...temps)} color="var(--ctp-peach)" thresholds={TEMP_THRESHOLDS} title={sparkTitle(t("Temperature"), temps, (v) => `${v.toFixed(0)} °C`, span)} />}
          </dd>
        </>
      )}
      {clock && (
        <>
          <dt>{t("CPU clock")}</dt>
          <dd>{clock}</dd>
        </>
      )}
      {h.throttle && (
        <>
          <dt>{t("Throttling")}</dt>
          <dd className={h.throttle.now.length ? "health-body-warn" : undefined}>{throttleWords(h.throttle)}</dd>
        </>
      )}
      {h.battery && (
        <>
          <dt>{t("Battery")}</dt>
          <dd className={batteryLow(h.battery) ? "health-body-warn" : undefined}>{batteryWords(h.battery)}</dd>
        </>
      )}
    </>
  );
}
