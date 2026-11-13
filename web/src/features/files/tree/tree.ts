// The project tree's pure part: where an entry lies, what its git mark says, how a row reads, what its menu offers and
// what a file handed to an agent says.

import { joinPath, separatorOf } from "@/lib/pathname";
import { t } from "@/lib/i18n";

export type Mark = "A" | "M" | "D" | "U" | "?";

export type TreeEntry = { name: string; dir?: boolean; link?: boolean; mark?: Mark; ignored?: boolean; changes?: boolean; gone?: boolean };

export type TreeListing = { path: string; entries: TreeEntry[]; more?: boolean; repo: boolean; git?: string };

export const PER_FOLDER = 500;
export const INDENT = 14; // px a level
export const REFRESH_EVERY = 10_000;
export const CHAT_PAUSE = 3_000; // a busy agent's conversation moves every second: the tree reads again at most this often

// childOf is an entry's path relative to the tree's root, with forward slashes
export const childOf = (rel: string, name: string): string => (rel ? `${rel}/${name}` : name);

// absoluteOf is the entry's path on its machine, each part joined with the folder's own separator
export const absoluteOf = (dir: string, rel: string): string => rel.split("/").filter(Boolean).reduce(joinPath, dir);

// relativeOf is the path as the folder's own spelling writes it under the workspace: a Windows one with backslashes
export const relativeOf = (dir: string, rel: string): string => (separatorOf(dir) === "\\" ? rel.replaceAll("/", "\\") : rel);

export type Tone = "added" | "changed" | "deleted" | "conflict" | "";

// toneOf is the colour a row's name wears: green new or added, amber changed (a folder holding changes too), red gone or in conflict
export function toneOf(entry: TreeEntry): Tone {
  switch (entry.mark) {
    case "A":
    case "?":
      return "added";
    case "M":
      return "changed";
    case "D":
      return "deleted";
    case "U":
      return "conflict";
  }
  return entry.dir && entry.changes ? "changed" : "";
}

// markWords says what a mark means, for the letter's tooltip
export function markWords(mark: Mark | undefined): string {
  switch (mark) {
    case "A":
      return t("Added, staged for the next commit");
    case "M":
      return t("Changed since HEAD");
    case "D":
      return t("Deleted since HEAD");
    case "U":
      return t("In conflict: a merge stopped on it");
    case "?":
      return t("New, not yet in git");
  }
  return "";
}

// rowTitle is a row's tooltip: its path, what git says of it, and that it is ignored or a link
export function rowTitle(entry: TreeEntry, rel: string): string {
  const said = [markWords(entry.mark), entry.dir && entry.changes && !entry.mark ? t("Holds changes") : "", entry.ignored ? t("Ignored by git") : "", entry.link ? t("A symbolic link") : ""].filter(Boolean);
  return [rel, ...said].join("\n");
}

export type RowAction = "open" | "editor" | "changes" | "copy-relative" | "copy-full" | "send";

// rowActions is what a row's menu offers: a viewer reads and copies alone; the editor is absent on a machine without one
export function rowActions(entry: TreeEntry, { viewOnly, editor }: { viewOnly: boolean; editor: boolean }): RowAction[] {
  const actions: RowAction[] = [];
  if (!entry.dir) actions.push("open");
  if (editor && !viewOnly && !entry.gone) actions.push("editor");
  if (!entry.dir && !viewOnly && entry.mark && entry.mark !== "?") actions.push("changes");
  actions.push("copy-relative", "copy-full");
  if (!viewOnly) actions.push("send");
  return actions;
}

// fileMessage is what goes to the agent: the instruction, then the path — never the file's text, so it reads the file as
// it is by then
export const fileMessage = (instruction: string, path: string, dir: boolean): string => `${instruction.trim()}\n\n${dir ? "Folder" : "File"}: ${path}`;

// moreWords is the line under a folder cut at the cap
export const moreWords = (): string => t("Showing the first {n}", { n: PER_FOLDER });
