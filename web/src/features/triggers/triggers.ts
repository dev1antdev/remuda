// The triggers' words and the dialog's pure part: a rule as a line, where it looks, which ones a folder sees, the
// form's checks, and the dev servers offered as previews once each.

import type { Machine, ServeOffer } from "@/features/roster/api";
import { t } from "@/lib/i18n";
import { absolutePath } from "@/lib/pathname";
import type { Action, Match, Rule } from "./api";

export const MAX_PATTERN = 400;
export const MAX_NAME = 80;

// actionOptions is the actions a rule can take, as the form offers them
export const actionOptions = (): { value: Action; label: string; hint: string }[] => [
  { value: "push", label: t("Send a push"), hint: t("a notification to your devices, and a row in the activity") },
  { value: "feed", label: t("Note it in the activity"), hint: t("a row in the activity, nothing on your devices") },
  { value: "preview", label: t("Offer a preview"), hint: t("the port the line names, offered in the corner once it listens") },
  { value: "task", label: t("Move the task"), hint: t("the task whose agent runs in the pane, to the column named") },
];

export const matchOptions = (): { value: Match; label: string }[] => [
  { value: "substring", label: t("Text, as written") },
  { value: "regex", label: t("Regular expression") },
];

// Scope is where a rule looks: everywhere, every folder of one machine, or one folder of it
export type Scope = "everywhere" | "machine" | "folder";

export const scopeOf = (rule: Rule): Scope => (rule.dir ? "folder" : rule.machine ? "machine" : "everywhere");

// actionWords is a rule's action as a phrase
export function actionWords(rule: Rule): string {
  switch (rule.action) {
    case "push":
      return t("push");
    case "feed":
      return t("activity");
    case "preview":
      return t("preview");
    case "task":
      return t("task → {column}", { column: rule.column ?? "" });
  }
}

// scopeWords says where a rule looks, the machine by its label
export function scopeWords(rule: Rule, machineLabel: (id: string) => string): string {
  switch (scopeOf(rule)) {
    case "folder":
      return t("in {folder} on {machine}", { folder: rule.dir ?? "", machine: machineLabel(rule.machine ?? "") });
    case "machine":
      return t("everywhere on {machine}", { machine: machineLabel(rule.machine ?? "") });
  }
  return t("everywhere");
}

// ruleWords is a rule's second line: what it looks for, where, and what it does
export function ruleWords(rule: Rule, machineLabel: (id: string) => string): string {
  const looks = rule.match === "regex" ? `/${rule.pattern}/` : `“${rule.pattern}”`;
  const parts = [looks, scopeWords(rule, machineLabel), actionWords(rule)];
  if (!rule.enabled) parts.push(t("off"));
  return parts.join(" · ");
}

// within says whether a folder is dir or under it
const within = (path: string, dir: string): boolean => path === dir || path.startsWith(dir.replace(/\/$/, "") + "/");

// looksAt says whether a rule looks at a folder of a machine, as the server's Covers does, switched on or not
export function looksAt(rule: Rule, machine: string, dir: string): boolean {
  if (rule.machine && rule.machine !== machine) return false;
  return !rule.dir || (dir !== "" && within(dir, rule.dir));
}

// draftCheck is what keeps the form from saving, in words; "" when it may
export function draftCheck(rule: Rule): string {
  if (!rule.name.trim()) return t("Name the trigger.");
  if (!rule.pattern.trim()) return t("Say what to look for.");
  if (rule.pattern.length > MAX_PATTERN) return t("A pattern is {n} characters at most.", { n: MAX_PATTERN });
  if (/[\r\n]/.test(rule.pattern)) return t("herdr reads one line at a time: a pattern holds no line break.");
  if (rule.action === "task" && !rule.column?.trim()) return t("Name the column the task moves to.");
  if (scopeOf(rule) === "folder" && !absolutePath(rule.dir ?? "")) return t("The folder is an absolute path.");
  return "";
}

// newRule is a fresh rule: for the folder the dialog was opened on, else everywhere
export function newRule(machine?: string, dir?: string): Rule {
  return { name: "", machine: dir ? machine : "", dir: dir ?? "", match: "substring", pattern: "", action: "push", enabled: true };
}

// withScope is a rule moved to another scope: the machine and folder the dialog knows, or none
export function withScope(rule: Rule, scope: Scope, machine: string, dir: string): Rule {
  switch (scope) {
    case "folder":
      return { ...rule, machine, dir: rule.dir || dir };
    case "machine":
      return { ...rule, machine, dir: "" };
  }
  return { ...rule, machine: "", dir: "" };
}

// Offered is a dev server to offer: the machine, the pane, its workspace's name, and the offer
export type Offered = { machine: string; pane: string; workspace: string; offer: ServeOffer; key: string };

// OFFER_FRESH is how old an offer may be and still go up in the corner: a page opened later says nothing of it
export const OFFER_FRESH = 10 * 60_000;
// MAX_SEEN is how many offers the device remembers having shown
export const MAX_SEEN = 30;

// offerKey names one offer: the pane, the port, and when it was made
export const offerKey = (machine: string, pane: string, offer: ServeOffer): string => `${machine}|${pane}|${offer.port}|${offer.at}`;

// offersToShow is every fresh offer on a connected machine this device has not shown yet
export function offersToShow(machines: Machine[], seen: string[], now: number): Offered[] {
  const out: Offered[] = [];
  for (const m of machines) {
    if (m.state !== "connected") continue;
    for (const session of m.sessions)
      for (const tab of session.tabs)
        for (const pane of tab.panes) {
          const offer = pane.serves;
          if (!offer || now - Date.parse(offer.at) > OFFER_FRESH) continue;
          const key = offerKey(m.id, pane.id, offer);
          if (!seen.includes(key)) out.push({ machine: m.id, pane: pane.id, workspace: session.name, offer, key });
        }
  }
  return out;
}

// rememberOffer keeps an offer as shown, the newest MAX_SEEN
export const rememberOffer = (seen: string[], key: string): string[] => [...seen.filter((k) => k !== key), key].slice(-MAX_SEEN);

// offerWords is the toast's line: "web serves on :5173 (vite)"
export function offerWords(o: Offered): string {
  const what = o.offer.label ? ` (${o.offer.label})` : "";
  return t("{workspace} serves on :{port}{what}", { workspace: o.workspace, port: o.offer.port, what });
}
