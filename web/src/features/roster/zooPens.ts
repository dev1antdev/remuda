// The zoo's pens and its sky: the creatures stand by machine, each pen under a sign with the machine's name, and the
// pen's weather is the machine's body as its vitals read it — sun and sweat when its CPU runs warm or hot, a storm for
// under-voltage, boxes piled up when its disk is nearly full, dust when its load is past its cores, a lamp while it is
// kept awake; over them all the sky of the viewer's hour. The pure part; app/zoo draws them.

import { t } from "@/lib/i18n";
import type { Awake, Health } from "./api";
import { HOT, WARM } from "./body";

export type Pen<T> = { machineId: string; tiles: T[] };

// pensOf groups the creatures by machine, the machines in the order their first creature stands, each pen's in theirs
export function pensOf<T extends { machine: { id: string } }>(tiles: T[]): Pen<T>[] {
  const pens = new Map<string, T[]>();
  for (const tile of tiles) {
    const pen = pens.get(tile.machine.id);
    if (pen) pen.push(tile);
    else pens.set(tile.machine.id, [tile]);
  }
  return [...pens].map(([machineId, members]) => ({ machineId, tiles: members }));
}

export type Heat = "" | "warm" | "hot";
export type Weather = { heat: Heat; storm: boolean; boxes: boolean; dust: boolean; lamp: boolean };

export const CLEAR: Weather = { heat: "", storm: false, boxes: false, dust: false, lamp: false };

// BOXES_AT is the free share of the disk, in percent, under which the boxes pile up: the ⓘ's own amber line
export const BOXES_AT = 10;

// weatherOf is a pen's weather by its machine's vitals and its keep-awake hold
export function weatherOf(health: Health | undefined, awake: Awake | undefined): Weather {
  if (!health) return { ...CLEAR, lamp: !!(awake?.on && awake.held) };
  const temp = health.temp_c ?? 0;
  const free = 100 - (health.disk_used_pct ?? 0);
  return {
    heat: temp >= HOT ? "hot" : temp >= WARM ? "warm" : "",
    storm: !!health.throttle?.now.includes("under-voltage"),
    boxes: health.disk_used_pct > 0 && free <= BOXES_AT,
    dust: !!health.cores && health.load1 > health.cores,
    lamp: !!(awake?.on && awake.held),
  };
}

// weatherWords is the pen's sign in words: what the weather means, else the machine's figures
export function weatherWords(health: Health | undefined, weather: Weather): string {
  const words: string[] = [];
  if (weather.storm) words.push(t("under-voltage"));
  if (health?.temp_c) words.push(t("{temp} °C", { temp: health.temp_c.toFixed(0) }));
  if (health && (weather.dust || health.load1 > 0)) words.push(t("load {load}", { load: health.load1.toFixed(1) }));
  if (health && weather.boxes) words.push(t("disk {pct}% full", { pct: health.disk_used_pct.toFixed(0) }));
  if (weather.lamp) words.push(t("kept awake"));
  return words.join(" · ");
}

export type Sky = "dawn" | "day" | "dusk" | "night";

// skyOf is the sky for an hour of the viewer's clock: dawn from 5, day from 8, dusk from 18, night from 21
export function skyOf(hour: number): Sky {
  if (hour >= 5 && hour < 8) return "dawn";
  if (hour >= 8 && hour < 18) return "day";
  if (hour >= 18 && hour < 21) return "dusk";
  return "night";
}
