// The palette's list: every pane as a row to jump to, every command the page offers, and their order by
// what was typed. Pure: the rows are built from the roster, the ranking from words.

import type { FolderIconRef, Machine, Session } from "@/features/roster/api";
import { machineHue } from "@/features/roster/hue";
import { displayOf, markOf, recentRows, rowLabel, stateWords, type Display, type RadarPrefs } from "@/features/roster/radar";
import { basename } from "@/lib/text";
import { folderOf } from "@/features/roster/workspaces";
import type { IconName } from "@/ui/icons";
import { VENDOR_NAME } from "@/ui/brand";

export type Selection = { machineId: string; paneId: string };

export type PaneRow = { machineId: string; paneId: string; agent: string; display: Display; alert?: string; mark: string; current: boolean; folder: { name: string; icon?: FolderIconRef } };

export type Entry = {
  id: string;
  kind: "pane" | "command";
  title: string;
  hint?: string; // in grey after the title: the workspace and machine, or what the command does
  detail?: string; // at the right edge: the agent's state
  keywords: string[]; // what else the typed words may hit
  pane?: PaneRow;
  hue?: number; // the pane's machine, as a dot before the hint; none for the local one
  icon?: IconName;
  danger?: boolean;
  always?: boolean; // listed whatever was typed, after the rest
  run: () => void;
};

// Masked is what a pane of a folder hidden while the screen is shared reads as: its folder's and machine's neutral names
export type Masked = { folder: string; machine: string };

// paneEntries is every pane on every machine, each machine's liveliest first, shells at its end; a pane masked by the
// screen-sharing mode goes by its agent and the neutral names, its own title, folder and tab name left out.
export function paneEntries(
  machines: Machine[],
  prefs: RadarPrefs,
  now: number,
  step: number,
  current: Selection | null,
  open: (machineId: string, paneId: string) => void,
  masked: (machine: Machine, session: Session) => Masked | null = () => null,
): Entry[] {
  const out: Entry[] = [];
  for (const machine of machines) {
    for (const { pane, tab, session } of recentRows(machine, { ...prefs, shells: true })) {
      const display: Display = pane.agent ? displayOf(pane, prefs, now) : "idle";
      const alert = pane.radar?.alert;
      const mask = masked(machine, session);
      const agentName = pane.agent ? (VENDOR_NAME[pane.agent] ?? pane.agent) : "shell";
      out.push({
        id: `pane:${machine.id}|${pane.id}`,
        kind: "pane",
        title: mask ? agentName : rowLabel(pane, tab, session.name, prefs),
        hint: mask ? `${mask.folder}${machine.id !== "local" ? ` · ${mask.machine}` : ""}` : `${session.name}${machine.id !== "local" ? ` · ${machine.label}` : ""}`,
        hue: machineHue(machine.id) ?? undefined,
        detail: pane.agent ? (alert ?? stateWords(display, pane.radar?.last_active, now, pane.radar?.since)) : "shell",
        keywords: mask ? [mask.folder, mask.machine, pane.agent || "shell"] : [session.name, machine.label, pane.agent || "shell", basename(pane.cwd), tab.named ? tab.label : ""].filter(Boolean),
        pane: {
          machineId: machine.id,
          paneId: pane.id,
          agent: pane.agent,
          display,
          alert,
          mark: alert ? "!" : pane.agent ? markOf(display, step) : "$",
          current: current?.machineId === machine.id && current.paneId === pane.id,
          folder: mask ? { name: mask.folder } : { name: basename(folderOf(session)) || session.name, icon: session.icon },
        },
        run: () => open(machine.id, pane.id),
      });
    }
  }
  return out;
}

// rank keeps the entries every typed word hits, the best hits first, an agent before a shell at the same
// score, ties in the given order; nothing typed keeps them all.
export function rank(entries: Entry[], query: string): Entry[] {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean);
  if (words.length === 0) return entries;
  const hits: { entry: Entry; score: number; index: number }[] = [];
  entries.forEach((entry, index) => {
    const score = scoreOf(entry, words);
    if (score > 0) hits.push({ entry, score, index });
  });
  hits.sort((a, b) => b.score - a.score || shell(a.entry) - shell(b.entry) || a.index - b.index);
  return hits.map((h) => h.entry);
}

const shell = (entry: Entry) => (entry.pane && !entry.pane.agent ? 1 : 0);

// scoreOf adds up each word's best hit, a hit in the title counting double; a word that hits nothing drops the entry.
// A shell counts half: its title is only its folder's name, which the agent beside it carries as a keyword.
function scoreOf(entry: Entry, words: string[]): number {
  const title = entry.title.toLowerCase();
  const keywords = entry.keywords.map((k) => k.toLowerCase());
  const weight = shell(entry) ? 0.5 : 1;
  let total = 0;
  for (const word of words) {
    let best = 2 * hit(title, word, true);
    for (const keyword of keywords) best = Math.max(best, hit(keyword, word, false));
    if (best === 0) return 0;
    total += weight * best;
  }
  return total;
}

// hit grades where a word sits in a text: the whole of it, its start, the start of a word in it, anywhere, or — in a title only — as initials.
function hit(text: string, word: string, loose: boolean): number {
  if (text === word) return 6;
  if (text.startsWith(word)) return 5;
  if (new RegExp(`[^\\p{L}\\p{N}]${escape(word)}`, "u").test(text)) return 4;
  if (text.includes(word)) return 3;
  return loose && initials(text, word) ? 1 : 0;
}

const escape = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");

// initials: each letter of the word starts a later word of the text ("gw" for "Go to the Wall").
function initials(text: string, word: string): boolean {
  const starts = text.split(/[^\p{L}\p{N}]+/u).filter(Boolean).map((w) => w[0]!);
  let at = 0;
  for (const ch of word) {
    at = starts.indexOf(ch, at);
    if (at < 0) return false;
    at++;
  }
  return true;
}
