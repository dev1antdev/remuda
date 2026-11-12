// A machine's body as its vitals read it: memory, the CPU's temperature and clock, what a Raspberry Pi's firmware held it
// back for — the words of the ⓘ's rows and of the Server place, the thresholds of their sparklines, and the tone the
// Server place's rail icon wears. Pure, so the rules are tested.

import type { Battery, Health, Throttle } from "./api";
import type { Threshold } from "./spark";
import { t } from "@/lib/i18n";

const gb = (mb: number) => (mb / 1024).toFixed(mb >= 10 * 1024 ? 0 : 1);

// memWords: "1.1 of 7.6 GB used · 6.5 GB available", with the swap when there is one
export function memWords(h: Health): string {
  if (!h.mem_total_mb) return "";
  const avail = h.mem_avail_mb ?? 0;
  const used = Math.max(0, h.mem_total_mb - avail);
  const words = [t("{used} of {total} GB used", { used: gb(used), total: gb(h.mem_total_mb) })];
  if (h.mem_avail_mb !== undefined) words.push(t("{avail} GB available", { avail: gb(avail) }));
  if (h.swap_total_mb) words.push(t("swap {used} of {total} GB", { used: gb(h.swap_used_mb ?? 0), total: gb(h.swap_total_mb) }));
  return words.join(" · ");
}

// memUsedPct is the share of the memory in use, null when it was not read
export function memUsedPct(h: Health): number | null {
  if (!h.mem_total_mb || h.mem_avail_mb === undefined) return null;
  return Math.round((1000 * (h.mem_total_mb - h.mem_avail_mb)) / h.mem_total_mb) / 10;
}

// tempWords: "62.8 °C · critical at 110 °C"
export function tempWords(h: Health): string {
  if (!h.temp_c) return "";
  const now = t("{temp} °C", { temp: h.temp_c.toFixed(1) });
  return h.temp_crit_c ? `${now} · ${t("critical at {temp} °C", { temp: h.temp_crit_c.toFixed(0) })}` : now;
}

// clockWords: "1.3 of 1.8 GHz"
export function clockWords(h: Health): string {
  if (!h.freq_mhz) return "";
  const ghz = (mhz: number) => (mhz / 1000).toFixed(1);
  return h.freq_max_mhz ? t("{now} of {max} GHz", { now: ghz(h.freq_mhz), max: ghz(h.freq_max_mhz) }) : t("{now} GHz", { now: ghz(h.freq_mhz) });
}

const THROTTLE_WORDS: Record<string, string> = {
  "under-voltage": "under-voltage",
  "frequency capped": "frequency capped",
  throttled: "throttled",
  "soft temperature limit": "soft temperature limit",
};
const said = (what: string[]) => what.map((w) => t(THROTTLE_WORDS[w] ?? w)).join(", ");

// throttleWords: what holds now, what held since boot, and — with the alarm alone — that the firmware's word needs root
export function throttleWords(throttle: Throttle): string {
  const now = throttle.now.length ? t("now: {what}", { what: said(throttle.now) }) : t("none now");
  if (!throttle.word) return `${now} · ${t("under-voltage alarm only: the firmware's word needs root here (vcgencmd)")}`;
  return throttle.since?.length ? `${now} · ${t("since boot: {what}", { what: said(throttle.since) })}` : `${now} · ${t("none since boot")}`;
}

// LOW_BATTERY is where a machine on its battery stops taking a fleet queue's agents (internal/placement's own)
export const LOW_BATTERY = 20;

// batteryWords: "85 % · on battery", "92 % · charging", "100 % · on power"
export function batteryWords(battery: Battery): string {
  const state = battery.discharging ? t("on battery") : battery.charging ? t("charging") : t("on power");
  return `${battery.percent} % · ${state}`;
}

// batteryLow says whether the machine runs on a battery nearly empty
export const batteryLow = (battery: Battery): boolean => !!battery.discharging && battery.percent <= LOW_BATTERY;

// WARM is where a CPU gets a look and HOT where it is a concern: a Raspberry Pi 4 starts capping its clock at 80 °C
export const WARM = 75;
export const HOT = 85;

export const TEMP_THRESHOLDS: Threshold[] = [
  { value: WARM, tone: "amber" },
  { value: HOT, tone: "red" },
];

// MEM_THRESHOLDS are the memory in use that wants a look (90 %) and that is a concern (95 %)
export const MEM_THRESHOLDS: Threshold[] = [
  { value: 90, tone: "amber" },
  { value: 95, tone: "red" },
];

export type BodyConcern = { text: string; bad: boolean };

// bodyConcerns are what the body says is wrong: under-voltage and heat are bad, a capped clock, warmth and tight memory a look
export function bodyConcerns(h: Health | undefined): BodyConcern[] {
  if (!h) return [];
  const out: BodyConcern[] = [];
  const throttle = h.throttle;
  if (throttle?.now.includes("under-voltage")) out.push({ text: t("Under-voltage: the power supply is too weak"), bad: true });
  const held = throttle?.now.filter((w) => w !== "under-voltage") ?? [];
  if (held.length) out.push({ text: t("Held back now: {what}", { what: said(held) }), bad: false });
  else if (throttle?.since?.length) out.push({ text: t("Held back since boot: {what}", { what: said(throttle.since) }), bad: false });
  if (h.temp_c && h.temp_c >= HOT) out.push({ text: t("Hot: {temp} °C", { temp: h.temp_c.toFixed(0) }), bad: true });
  else if (h.temp_c && h.temp_c >= WARM) out.push({ text: t("Warm: {temp} °C", { temp: h.temp_c.toFixed(0) }), bad: false });
  const mem = memUsedPct(h);
  if (mem !== null && mem >= 95) out.push({ text: t("Memory nearly full: {pct}% in use", { pct: mem.toFixed(0) }), bad: true });
  else if (mem !== null && mem >= 90) out.push({ text: t("Memory tight: {pct}% in use", { pct: mem.toFixed(0) }), bad: false });
  return out;
}
