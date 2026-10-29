// What the composer offers at the caret, pure: the token the caret sits in (a slash command at the very start of
// the message, an @ after a space or the start), the lists ranked as the palette ranks its rows, and the draft with
// a pick put in.

import { rank, type Entry } from "@/features/palette/rank";
import { t } from "@/lib/i18n";

export type Command = { name: string; description?: string; source: "builtin" | "user" | "project" };
export type Token = { kind: "command" | "path"; start: number; end: number; query: string };
export type Item = { id: string; title: string; hint?: string; tag?: string; insert: string };

// MAX_SHOWN rows are listed at most; the rest wait for more letters
export const MAX_SHOWN = 12;

// tokenAt is the token under the caret: "/name" from the draft's first character to the caret with no space in it,
// or "@path" after the start or a space; nothing once a space ends the word, so a sent command's arguments see no list
export function tokenAt(draft: string, caret: number): Token | null {
  const before = draft.slice(0, Math.max(0, Math.min(caret, draft.length)));
  const slash = /^\/(\S*)$/.exec(before);
  if (slash) return { kind: "command", start: 0, end: before.length, query: slash[1]! };
  const at = /(?:^|\s)@([^\s@]*)$/.exec(before);
  if (at) return { kind: "path", start: before.length - at[1]!.length - 1, end: before.length, query: at[1]! };
  return null;
}

// applyPick replaces the token with the pick and a space (the one already there kept), the caret landing after it
export function applyPick(draft: string, token: Token, insert: string): { draft: string; caret: number } {
  const after = draft.slice(token.end);
  const text = after.startsWith(" ") ? insert : `${insert} `;
  return { draft: draft.slice(0, token.start) + text + after, caret: token.start + insert.length + 1 };
}

// sourceWords says where a command comes from, nothing for the agent's own
export function sourceWords(source: Command["source"]): string {
  if (source === "user") return t("yours");
  if (source === "project") return t("this project");
  return "";
}

const entries = new WeakMap<object, Entry[]>();

// commandItems ranks the commands by what was typed after the slash: the name counts double, the description too
export function commandItems(list: Command[], query: string): Item[] {
  let all = entries.get(list);
  if (!all) {
    all = list.map((c) => ({ id: `${c.source}:${c.name}`, kind: "command", title: c.name, hint: c.description, keywords: c.description ? [c.description] : [], run: () => {} }));
    entries.set(list, all);
  }
  return rank(all, query)
    .slice(0, MAX_SHOWN)
    .map((e) => {
      const c = list[all!.indexOf(e)]!;
      return { id: e.id, title: `/${c.name}`, hint: c.description, tag: sourceWords(c.source), insert: `/${c.name}` };
    });
}

// pathItems ranks the folder's files by what was typed after the @: the whole path, its file name as a keyword
export function pathItems(list: string[], query: string): Item[] {
  let all = entries.get(list);
  if (!all) {
    all = list.map((p) => ({ id: p, kind: "command", title: p, keywords: [p.slice(p.lastIndexOf("/") + 1)], run: () => {} }));
    entries.set(list, all);
  }
  const ranked = query ? rank(all, query) : all;
  return ranked.slice(0, MAX_SHOWN).map((e) => ({ id: e.id, title: e.title, insert: `@${e.title}` }));
}

// modelCommand is what Claude Code is told to switch its model: its own /model with the id
export const modelCommand = (id: string): string => `/model ${id.trim()}`;
