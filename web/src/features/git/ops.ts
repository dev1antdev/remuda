// What each of git's writes asks before it runs, after git-graph's dialogs: names to type, a choice to pick, a box to
// tick, the words of what happens, red where it loses or rewrites something; then the request the server takes and the
// toast after it. Only a checkout of a local branch with nothing uncommitted runs at once.

import type { Change, OpRequest } from "./api";
import { current, short, type Action, type Context } from "./menus";
import { t, tn, tc } from "@/lib/i18n";

export type Input = { id: string; label: string; value: string; optional?: boolean; text?: boolean };
export type Check = { id: string; label: string; value: boolean };
export type Option = { value: string; label: string; detail: string; danger?: boolean };
export type Choice = { id: string; label: string; value: string; options: Option[] };
export type Spec = { title: string; detail: string; inputs?: Input[]; checks?: Check[]; choice?: Choice; confirm: string; danger?: boolean; lose?: string[] };
export type Values = Record<string, string | boolean>;

// Writing is an action that changes the checkout; copy and open are the menu's own
export type Writing = Exclude<Action, { type: "copy" } | { type: "changes" }>;

const LOSE_SHOWN = 8;

const mergeNote = (merge: boolean) => (merge ? " " + t("It is a merge commit: its changes against its first parent are used.") : "");

// tracked is the uncommitted files a hard reset discards: every tracked change, the untracked files staying
const tracked = (changes: Change[] | undefined) => (changes ?? []).map((c) => c.path);

// loseWords names the files a hard reset discards, the first few and the rest counted
export function loseWords(paths: string[]): string {
  if (!paths.length) return t("Nothing uncommitted is lost.");
  const named = paths.slice(0, LOSE_SHOWN).join(", ");
  const rest = paths.length - LOSE_SHOWN;
  return rest > 0
    ? tn(paths.length, "It discards the uncommitted changes to {n} file: {named} and {rest} more.", "It discards the uncommitted changes to {n} files: {named} and {rest} more.", { named, rest })
    : tn(paths.length, "It discards the uncommitted changes to {n} file: {named}.", "It discards the uncommitted changes to {n} files: {named}.", { named });
}

// asksFirst says whether an action shows its dialog before it runs
export const asksFirst = (action: Writing, ctx: Context): boolean => !(action.type === "checkout" && ctx.dirty === 0);

// specFor is the dialog an action shows; forced is the second word a branch delete asks when git said it is not merged
export function specFor(action: Writing, ctx: Context, changes?: Change[], forced = false): Spec {
  const cur = current(ctx);
  switch (action.type) {
    case "checkout":
      return {
        title: t("Checkout branch"),
        detail: tn(ctx.dirty, "Check out {branch}? {n} file is uncommitted: git carries it over, or refuses when the checkout would overwrite it.", "Check out {branch}? {n} files are uncommitted: git carries them over, or refuses when the checkout would overwrite one.", { branch: action.name }),
        confirm: t("Checkout"),
      };
    case "checkout-remote":
      return { title: t("Checkout remote branch"), detail: t("Create a local branch that tracks {name}, and check it out.", { name: action.name }), inputs: [{ id: "name", label: t("Local branch name"), value: action.branch }], confirm: t("Checkout") };
    case "checkout-commit":
      return { title: t("Checkout commit"), detail: t("Check out commit {hash}? You will be on a detached HEAD: new commits belong to no branch until you create one.", { hash: short(action.hash) }), confirm: t("Checkout") };
    case "branch-create":
      return { title: t("Create branch"), detail: t("At commit {hash}.", { hash: short(action.hash) }), inputs: [{ id: "name", label: t("Branch name"), value: "" }], checks: [{ id: "checkout", label: t("Check out the new branch"), value: false }], confirm: t("Create") };
    case "branch-rename":
      return { title: t("Rename branch"), detail: t("Rename {name}.", { name: action.name }), inputs: [{ id: "name", label: t("New name"), value: action.name }], confirm: t("Rename") };
    case "branch-delete":
      return forced
        ? { title: t("Delete branch"), detail: t("{name} is not merged into {current} or its upstream: the commits only it holds are lost. Delete it anyway?", { name: action.name, current: cur }), confirm: t("Delete anyway"), danger: true }
        : { title: t("Delete branch"), detail: t("Delete the local branch {name}?", { name: action.name }), confirm: t("Delete"), danger: true };
    case "tag-add":
      return {
        title: t("Add tag"),
        detail: t("At commit {hash}.", { hash: short(action.hash) }),
        inputs: [
          { id: "name", label: t("Tag name"), value: "" },
          { id: "message", label: t("Message (makes an annotated tag)"), value: "", optional: true, text: true },
        ],
        confirm: t("Add tag"),
      };
    case "tag-delete":
      return { title: t("Delete tag"), detail: t("Delete the local tag {name}?", { name: action.name }), confirm: t("Delete"), danger: true };
    case "merge":
      return { title: tc("git", "Merge"), detail: t("Merge {what} into {current}?", { what: action.what, current: cur }), checks: [{ id: "noff", label: t("Create a merge commit even if it could fast-forward"), value: false }], confirm: tc("git", "Merge") };
    case "rebase":
      return { title: tc("git", "Rebase"), detail: t("Rebase {current} on {what}? Its own commits are written again on top, with new hashes.", { current: cur, what: action.what }), confirm: tc("git", "Rebase") };
    case "cherry-pick":
      return { title: t("Cherry pick"), detail: t("Apply commit {hash} on top of {current}?", { hash: short(action.hash), current: cur }) + mergeNote(action.merge), confirm: t("Cherry pick") };
    case "revert":
      return { title: tc("git", "Revert"), detail: t("Add a commit to {current} that undoes {hash}?", { current: cur, hash: short(action.hash) }) + mergeNote(action.merge), confirm: tc("git", "Revert") };
    case "reset":
      return action.uncommitted
        ? {
            title: t("Reset uncommitted changes"),
            detail: t("Reset the working tree to HEAD."),
            choice: {
              id: "mode",
              label: tc("git", "Mode"),
              value: "mixed",
              options: [
                { value: "mixed", label: tc("git", "Mixed"), detail: t("Unstage everything, keep the changes") },
                { value: "hard", label: tc("git", "Hard"), detail: t("Discard every change to tracked files"), danger: true },
              ],
            },
            confirm: tc("git", "Reset"),
            lose: tracked(changes),
          }
        : {
            title: tc("git", "Reset"),
            detail: t("Move {current} to commit {hash}.", { current: cur, hash: short(action.hash) }),
            choice: {
              id: "mode",
              label: tc("git", "Mode"),
              value: "mixed",
              options: [
                { value: "soft", label: tc("git", "Soft"), detail: t("Keep every change, staged") },
                { value: "mixed", label: tc("git", "Mixed"), detail: t("Keep every change, unstaged") },
                { value: "hard", label: tc("git", "Hard"), detail: t("Discard every change"), danger: true },
              ],
            },
            confirm: tc("git", "Reset"),
            lose: tracked(changes),
          };
  }
}

// initialValues are a dialog's fields as it opens
export function initialValues(spec: Spec): Values {
  const values: Values = {};
  for (const input of spec.inputs ?? []) values[input.id] = input.value;
  for (const check of spec.checks ?? []) values[check.id] = check.value;
  if (spec.choice) values[spec.choice.id] = spec.choice.value;
  return values;
}

// dangerous says the dialog's button is red: the action loses something, or the option picked does
export function dangerous(spec: Spec, values: Values): boolean {
  return !!spec.danger || !!spec.choice?.options.find((o) => o.value === values[spec.choice!.id])?.danger;
}

// problemOf is what is wrong with the typed values, or "": a name never reaches git as an option
export function problemOf(spec: Spec, values: Values): string {
  for (const input of spec.inputs ?? []) {
    const text = String(values[input.id] ?? "").trim();
    if (input.optional) continue;
    if (!text) return t("{label} is empty.", { label: input.label });
    if (!input.text && (text.startsWith("-") || /\s/.test(text))) return t('{label} cannot start with "-" or hold a space.', { label: input.label });
  }
  return "";
}

// requestOf is what the server is asked to run, with HEAD and the branch as the graph showed them
export function requestOf(action: Writing, values: Values, ctx: Context, forced = false): OpRequest {
  const seen = { head: ctx.head, branch: ctx.branch };
  const name = String(values.name ?? "").trim();
  switch (action.type) {
    case "checkout":
      return { op: "checkout", ref: action.name, ...seen };
    case "checkout-remote":
      return { op: "checkout-remote", ref: action.name, name, ...seen };
    case "checkout-commit":
      return { op: "checkout-commit", sha: action.hash, ...seen };
    case "branch-create":
      return { op: "branch-create", sha: action.hash, name, checkout: !!values.checkout, ...seen };
    case "branch-rename":
      return { op: "branch-rename", ref: action.name, name, ...seen };
    case "branch-delete":
      return { op: "branch-delete", ref: action.name, force: forced, ...seen };
    case "tag-add":
      return { op: "tag-add", sha: action.hash, name, message: String(values.message ?? "").trim(), ...seen };
    case "tag-delete":
      return { op: "tag-delete", ref: action.name, ...seen };
    case "merge":
      return { op: "merge", ...(action.hash ? { sha: action.hash } : { ref: action.ref }), no_ff: !!values.noff, ...seen };
    case "rebase":
      return { op: "rebase", ...(action.hash ? { sha: action.hash } : { ref: action.ref }), ...seen };
    case "cherry-pick":
      return { op: "cherry-pick", sha: action.hash, ...seen };
    case "revert":
      return { op: "revert", sha: action.hash, ...seen };
    case "reset":
      return { op: "reset", sha: action.hash, mode: values.mode as "soft" | "mixed" | "hard", ...seen };
  }
}

// doneWords is the toast after it went
export function doneWords(action: Writing, values: Values): string {
  const name = String(values.name ?? "").trim();
  switch (action.type) {
    case "checkout":
      return t("Checked out {name}", { name: action.name });
    case "checkout-remote":
      return t("Checked out {name}, tracking {remote}", { name, remote: action.name });
    case "checkout-commit":
      return t("Checked out {hash}", { hash: short(action.hash) });
    case "branch-create":
      return t("Created branch {name}", { name });
    case "branch-rename":
      return t("Renamed {from} to {name}", { from: action.name, name });
    case "branch-delete":
      return t("Deleted branch {name}", { name: action.name });
    case "tag-add":
      return t("Tagged {hash} as {name}", { hash: short(action.hash), name });
    case "tag-delete":
      return t("Deleted tag {name}", { name: action.name });
    case "merge":
      return t("Merged {what}", { what: action.what });
    case "rebase":
      return t("Rebased on {what}", { what: action.what });
    case "cherry-pick":
      return t("Cherry-picked {hash}", { hash: short(action.hash) });
    case "revert":
      return t("Reverted {hash}", { hash: short(action.hash) });
    case "reset":
      return action.uncommitted ? t("Reset uncommitted changes ({mode})", { mode: String(values.mode) }) : t("Reset to {hash} ({mode})", { hash: short(action.hash), mode: String(values.mode) });
  }
}
