// The "When" of a schedule: a preset that writes the cron — a time of day, a minute of the hour, or every few minutes
// inside a window of hours on weekdays or every day — or the cron by hand, and under it the cron in words with its next
// runs as the host's clock has them.

import { useQuery } from "@/lib/query";
import { whenAhead } from "@/lib/time";
import { Select, useDebounced } from "@/ui";
import { fetchNext } from "./api";
import { minuteOnly, PRESETS, STEPS, type When } from "./cron";
import { cronWords, hostClockWords } from "./cronWords";
import { t } from "@/lib/i18n";

const pad = (n: number) => String(n).padStart(2, "0");

// hourOptions are the whole hours from `first` to `last` as a Select lists them, 24 read as the midnight that ends the day
const hourOptions = (first: number, last: number) => Array.from({ length: last - first + 1 }, (_, i) => ({ value: String(first + i), label: `${first + i}:00` }));

export function WhenFields({ when, onWhen, cron, onCron }: { when: When; onWhen: (patch: Partial<When>) => void; cron: string; onCron: (cron: string) => void }) {
  return (
    <>
      <div className="schedule-row">
        <label>
          {t("When")}
          <Select value={when.preset} onChange={(preset) => onWhen({ preset })} options={PRESETS.map((p) => ({ value: p.id, label: p.label }))} />
        </label>
        {(when.preset === "daily" || when.preset === "weekdays") && (
          <label>
            {t("At")}
            <input
              type="time"
              value={`${pad(when.hh)}:${pad(when.mm)}`}
              onChange={(e) => {
                const [h, m] = e.target.value.split(":").map(Number);
                if (h !== undefined && !Number.isNaN(h)) onWhen({ hh: h, mm: m && !Number.isNaN(m) ? m : 0 });
              }}
            />
          </label>
        )}
        {minuteOnly(when.preset) && (
          <label>
            {t("Minute")}
            <input type="number" min={0} max={59} value={when.mm} onChange={(e) => onWhen({ mm: Math.max(0, Math.min(59, Number(e.target.value) || 0)) })} />
          </label>
        )}
        {when.preset === "every" && (
          <>
            <label>
              {t("Every")}
              <Select value={String(when.step)} onChange={(step) => onWhen({ step: Number(step) })} options={STEPS.map((n) => ({ value: String(n), label: t("{n} min", { n }) }))} />
            </label>
            <label>
              {t("From")}
              <Select
                value={String(when.from)}
                onChange={(picked) => {
                  const from = Number(picked);
                  onWhen({ from, until: Math.max(when.until, from + 1) });
                }}
                options={hourOptions(0, 23)}
              />
            </label>
            <label>
              {t("Until")}
              <Select value={String(when.until)} onChange={(until) => onWhen({ until: Number(until) })} options={hourOptions(when.from + 1, 24)} />
            </label>
          </>
        )}
        <label className="grow">
          {t("Cron")}
          <input value={cron} onChange={(e) => onCron(e.target.value)} placeholder={t("minute hour day month weekday")} spellCheck={false} />
        </label>
      </div>
      {when.preset === "every" && (
        <label className="schedule-check">
          <input type="checkbox" checked={when.weekdays} onChange={(e) => onWhen({ weekdays: e.target.checked })} />
          {t("Weekdays only")}
        </label>
      )}
      <CronReading cron={cron} />
    </>
  );
}

// CronReading is the cron in words and its next runs on the host's clock, or the host's reason when it cannot read it
function CronReading({ cron }: { cron: string }) {
  const typed = useDebounced(cron.trim(), 300);
  const upcoming = useQuery(() => fetchNext(typed), [typed], { enabled: typed !== "" });
  if (typed === "") return null;
  if (upcoming.error) return <p className="schedule-note bad">{upcoming.error}</p>;
  const next = upcoming.data?.next;
  const parts = [cronWords(typed) ?? ""];
  if (next) parts.push(next.length ? t("next: {times}", { times: next.map((at) => whenAhead(at)).join(", ") }) : t("No run within a year"));
  if (next?.[0]) parts.push(hostClockWords(next[0], -new Date(next[0]).getTimezoneOffset()));
  return <p className="schedule-note">{parts.filter(Boolean).join(" · ")}</p>;
}
