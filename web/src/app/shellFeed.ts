// What the Mac app's menu bar shows: the agents that need the person as the sidebar orders them, each with the words
// it last said, and how many others work or sit idle. The pure part; useShellFeed sends it.

import type { Machine } from "@/features/roster/api";
import { displayOf, groups, rowLabel, type Group, type RadarPrefs } from "@/features/roster/radar";
import { t } from "@/lib/i18n";
import type { ShellRow } from "@/lib/shell";
import type { Mode } from "./store";
import { toneOf } from "./badge";

export type ShellFeed = { rows: ShellRow[]; working: number; idle: number };

// MAX_ROWS is how many agents the menu lists; the badge's count says the rest
export const MAX_ROWS = 12;

// shellRows walks every connected machine's agents in the sidebar's order: the ones that need the person become rows,
// the rest are counted as working or idle
export function shellRows(machines: Machine[], radar: RadarPrefs, now: number): ShellFeed {
  const feed: ShellFeed = { rows: [], working: 0, idle: 0 };
  const walk = (machine: Machine, group: Group) => {
    for (const row of group.rows) {
      const pane = row.pane;
      if (!pane.agent) continue;
      const tone = toneOf(pane, radar, now);
      const state = displayOf(pane, radar, now);
      if (!tone) {
        if (state === "working") feed.working++;
        else if (state === "idle" || state === "idle_fresh" || state === "idle_stale") feed.idle++;
        continue;
      }
      feed.rows.push({
        machine: machine.id,
        machineLabel: machine.label,
        pane: pane.id,
        workspace: row.session.name,
        title: rowLabel(pane, row.tab, row.session.name, radar),
        said: saidOf(tone, pane.radar?.alert, pane.said),
        tone,
        state,
      });
    }
    for (const child of group.children) walk(machine, child);
  };
  for (const machine of machines) {
    if (machine.state !== "connected") continue;
    for (const group of groups(machine, radar)) walk(machine, group);
  }
  feed.rows = feed.rows.slice(0, MAX_ROWS);
  return feed;
}

// saidOf is the row's second line: the question's wait, the watchdog's note, or the first line of the last answer
function saidOf(tone: ShellRow["tone"], alert: string | undefined, said: string | undefined): string {
  if (tone === "red") return t("waiting for an answer");
  if (tone === "amber" && alert) return alert;
  return said ?? "";
}

// sameFeed says whether two feeds would draw the same menu
export function sameFeed(a: ShellFeed, b: ShellFeed): boolean {
  if (a.working !== b.working || a.idle !== b.idle || a.rows.length !== b.rows.length) return false;
  return a.rows.every((row, i) => {
    const other = b.rows[i];
    if (!other) return false;
    return (
      row.machine === other.machine &&
      row.pane === other.pane &&
      row.machineLabel === other.machineLabel &&
      row.workspace === other.workspace &&
      row.title === other.title &&
      row.said === other.said &&
      row.tone === other.tone &&
      row.state === other.state
    );
  });
}

const MODE_NAMES: Record<string, Mode> = { agents: "herdr", herdr: "herdr", wall: "wall", tasks: "tasks", review: "review", code: "code" };

// modeOf reads a mode's name as the app says it, "agents" for the first mode; nothing for a name the page has no mode for
export function modeOf(name: unknown): Mode | null {
  return typeof name === "string" ? (MODE_NAMES[name] ?? null) : null;
}
