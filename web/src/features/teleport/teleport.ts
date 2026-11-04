// The pure part of the move dialog: which machine and checkout it starts on, the folder picked or typed, why a move
// cannot go yet.

import { t, tn } from "@/lib/i18n";
import type { Target } from "./api";

// TYPED is the folder choice that asks for a path instead of a listed checkout
export const TYPED = "__typed";

// firstTarget is the machine a move starts on: another machine with a clone of the repository, else this one's other
// clone, else the first other machine
export function firstTarget(targets: Target[], source: string): string {
  const withClone = (t: Target) => t.checkouts.some((c) => c.same);
  return (targets.find((t) => t.machine !== source && withClone(t)) ?? targets.find(withClone) ?? targets.find((t) => t.machine !== source) ?? targets[0])?.machine ?? "";
}

// folderChoices is the checkouts of the repository on the target, then a path to type
export function folderChoices(target: Target | undefined): { value: string; label: string; hint?: string }[] {
  const same = (target?.checkouts ?? []).filter((c) => c.same).map((c) => ({ value: c.dir, label: c.dir, hint: c.branch || undefined }));
  return [...same, { value: TYPED, label: t("Another folder…"), hint: t("a clone of this repository there") }];
}

// firstFolder is the checkout a target starts on: its first clone of the repository, else a typed path
export function firstFolder(target: Target | undefined): string {
  return target?.checkouts.find((c) => c.same)?.dir ?? TYPED;
}

// folderOf is the folder the move goes to: the listed checkout, or the path typed
export const folderOf = (choice: string, typed: string): string => (choice === TYPED ? typed.trim() : choice);

// whyNot is what stops the move, or "" when it can go
export function whyNot(target: Target | undefined, folder: string, agent: string): string {
  if (!target) return t("Pick a machine to move the work to");
  if (!folder) return t("Pick or type a clone of this repository on {machine}", { machine: target.label });
  if (!folder.startsWith("/")) return t("The folder must be a full path, from /");
  if (!agent) return t("Pick the agent that carries on");
  return "";
}

// targetHint is what a machine offers, for its row in the picker
export function targetHint(target: Target): string {
  if (target.error) return target.error;
  const clones = target.checkouts.filter((c) => c.same).length;
  return clones ? tn(clones, "{n} clone of this repository", "{n} clones of this repository") : t("no clone of this repository open");
}
