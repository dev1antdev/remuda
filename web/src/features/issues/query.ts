// The search the issues panel shows in its field, and the chips and Filters menu that rewrite it: GitHub's own
// qualifiers (is:open, assignee:@me, author:@me, no:assignee, label:, sort:) written into the visible text, so what is
// asked is always what is on screen.

import { t, tc } from "@/lib/i18n";

export const OPEN_QUERY = "is:issue is:open";
export const MINE_QUERY = "is:issue is:open assignee:@me";
export const PER_PAGE = 25;

const tidy = (q: string) => q.replace(/\s+/g, " ").trim();

// the query's words, a quoted phrase kept whole
export function tokensOf(q: string): string[] {
  return q.match(/(?:[^\s"]+|"[^"]*")+/g) ?? [];
}

export function hasToken(q: string, token: string): boolean {
  return tokensOf(q).some((word) => word.toLowerCase() === token.toLowerCase());
}

// toggleToken takes a qualifier out when it is there, else adds it at the end
export function toggleToken(q: string, token: string): string {
  if (hasToken(q, token)) return tidy(tokensOf(q).filter((word) => word.toLowerCase() !== token.toLowerCase()).join(" "));
  return tidy(`${q} ${token}`);
}

export type IssueState = "open" | "closed" | "all";

export function stateOf(q: string): IssueState {
  const words = tokensOf(q).map((word) => word.toLowerCase());
  if (words.includes("is:open") || words.includes("state:open")) return "open";
  if (words.includes("is:closed") || words.includes("state:closed")) return "closed";
  return "all";
}

const isState = (word: string) => /^(is|state):(open|closed)$/i.test(word);
const isSort = (word: string) => /^sort:/i.test(word);

// withState puts one state qualifier in place of any there; "all" takes it out
export function withState(q: string, state: IssueState): string {
  const rest = tokensOf(q).filter((word) => !isState(word));
  const at = Math.min(rest.findIndex((word) => /^is:issue$/i.test(word)) + 1, rest.length);
  if (state !== "all") rest.splice(at, 0, `is:${state}`);
  return tidy(rest.join(" "));
}

export const SORTS = ["updated-desc", "created-desc", "comments-desc"] as const;
export type Sort = (typeof SORTS)[number];

export function sortOf(q: string): string {
  return tokensOf(q).find(isSort)?.slice(5).toLowerCase() ?? "updated-desc";
}

export function withSort(q: string, sort: Sort): string {
  return tidy(`${tokensOf(q).filter((word) => !isSort(word)).join(" ")} sort:${sort}`);
}

// labelToken is the qualifier for one label, quoted when its name has a blank
export function labelToken(name: string): string {
  return /[\s"]/.test(name) ? `label:"${name.replace(/"/g, "")}"` : `label:${name}`;
}

export function toggleLabel(q: string, name: string): string {
  return toggleToken(q, labelToken(name));
}

export type Chip = "open" | "mine" | null;

// chipOf is the chip a query is exactly, so it reads as pressed
export function chipOf(q: string): Chip {
  const at = tidy(q).toLowerCase();
  if (at === OPEN_QUERY) return "open";
  if (at === MINE_QUERY) return "mine";
  return null;
}

// asked is what goes to the server: the query as typed, or the open issues for nothing
export function asked(q: string): string {
  return tidy(q) || OPEN_QUERY;
}

export type FilterRow = { heading: string } | { sep: true } | { id: string; label: string; hint?: string; checked: boolean; query: string };

// filterRows is the Filters menu for a query: the state, the people, the labels seen in the list, the order, a reset
export function filterRows(q: string, labels: string[]): FilterRow[] {
  const state = stateOf(q);
  const sort = sortOf(q);
  const rows: FilterRow[] = [
    { heading: t("State") },
    { id: "open", label: tc("issues", "Open"), checked: state === "open", query: withState(q, state === "open" ? "all" : "open") },
    { id: "closed", label: tc("issues", "Closed"), checked: state === "closed", query: withState(q, state === "closed" ? "all" : "closed") },
    { heading: t("People") },
    { id: "mine", label: t("Assigned to me"), hint: "assignee:@me", checked: hasToken(q, "assignee:@me"), query: toggleToken(q, "assignee:@me") },
    { id: "author", label: t("Created by me"), hint: "author:@me", checked: hasToken(q, "author:@me"), query: toggleToken(q, "author:@me") },
    { id: "nobody", label: t("No assignee"), hint: "no:assignee", checked: hasToken(q, "no:assignee"), query: toggleToken(q, "no:assignee") },
  ];
  if (labels.length > 0) {
    rows.push({ heading: t("Labels") });
    for (const name of labels) rows.push({ id: `label:${name}`, label: name, hint: labelToken(name), checked: hasToken(q, labelToken(name)), query: toggleLabel(q, name) });
  }
  rows.push(
    { heading: t("Sort") },
    { id: "updated", label: t("Recently updated"), checked: sort === "updated-desc", query: withSort(q, "updated-desc") },
    { id: "created", label: t("Newest"), checked: sort === "created-desc", query: withSort(q, "created-desc") },
    { id: "comments", label: t("Most commented"), checked: sort === "comments-desc", query: withSort(q, "comments-desc") },
    { sep: true },
    { id: "reset", label: t("Reset"), hint: OPEN_QUERY, checked: false, query: OPEN_QUERY },
  );
  return rows;
}

// labelsIn are the labels the list's issues wear, each once, the most worn first, at most max
export function labelsIn(items: { labels: { name: string }[] }[], max = 12): string[] {
  const count = new Map<string, number>();
  for (const item of items) for (const label of item.labels) count.set(label.name, (count.get(label.name) ?? 0) + 1);
  return [...count.entries()].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).slice(0, max).map(([name]) => name);
}

export function pageCount(total: number, per = PER_PAGE): number {
  return Math.max(1, Math.ceil(total / per));
}

export function pageOf<T>(items: T[], page: number, per = PER_PAGE): T[] {
  const at = Math.min(Math.max(0, page), pageCount(items.length, per) - 1);
  return items.slice(at * per, (at + 1) * per);
}

// pagerPages are the page numbers the pager shows: five around the one on screen
export function pagerPages(at: number, pages: number): number[] {
  const from = Math.max(0, Math.min(at - 2, pages - 5));
  return Array.from({ length: Math.min(5, pages) }, (_, k) => from + k);
}
