// The collision radar in words: another checkout of the repository changing the same files against the main branch —
// the sidebar card's and the wall tile's quiet mark, its tooltip, the Changes lens's notice and the push's order. The
// pure part, tested; CollisionMark.tsx draws the mark.

import { t, tn } from "@/lib/i18n";
import type { Collision, Overlap } from "./api";

// whereWords is one file both change and where: its lines in the base, the whole file, or only the file
export function whereWords(file: Overlap): string {
  if (!file.lines) return t("{path} (other lines)", { path: file.path });
  if (!file.from) return t("{path} (the whole file)", { path: file.path });
  if (file.from === file.to) return t("{path}, line {line}", { path: file.path, line: file.from });
  return t("{path}, lines {from}–{to}", { path: file.path, from: file.from, to: file.to ?? file.from });
}

// othersCount is how many files beside the first both change
export function othersCount(collision: Collision): number {
  return Math.max(0, collision.files.length - 1) + (collision.more ?? 0);
}

// markWords is the mark's line: "collides with web on src/auth.ts", the same lines said so
export function markWords(collision: Collision, peer: string): string {
  const file = collision.files[0]?.path ?? "";
  const more = othersCount(collision);
  if (collision.lines) return more ? t("same lines as {peer} in {file} and {n} more", { peer, file, n: more }) : t("same lines as {peer} in {file}", { peer, file });
  return more ? t("collides with {peer} on {file} and {n} more", { peer, file, n: more }) : t("collides with {peer} on {file}", { peer, file });
}

// whyWords is why one of two lands first
export function whyWords(why: Collision["why"]): string {
  switch (why) {
    case "pr":
      return t("it has a pull request open");
    case "rest":
      return t("its agents have finished");
    case "smaller":
      return t("its change is the smaller one");
  }
  return "";
}

// orderWords is which of the two lands first, from this workspace's side
export function orderWords(collision: Collision, peer: string): string {
  const why = whyWords(collision.why);
  if (collision.first) return why ? t("This one lands first: {why}; {peer} rebases on it.", { why, peer }) : t("This one lands first; {peer} rebases on it.", { peer });
  return why ? t("Land {peer} first: {why}; this one rebases on it.", { why, peer }) : t("Land {peer} first; this one rebases on it.", { peer });
}

// collisionWords is a collision whole, for a tooltip or the lens's notice: with whom, the files and where, the order
export function collisionWords(collision: Collision, peer: string): string[] {
  const head = collision.lines ? t("Changes the same lines as {peer}, against {main}:", { peer, main: collision.main }) : t("Changes the same files as {peer}, on other lines, against {main}:", { peer, main: collision.main });
  const files = collision.files.slice(0, MAX_FILES_SAID).map(whereWords);
  const rest = collision.files.length - files.length + (collision.more ?? 0);
  return [head, ...files.map((line) => `· ${line}`), ...(rest > 0 ? [tn(rest, "· and {n} more file", "· and {n} more files")] : []), orderWords(collision, peer)];
}

// MAX_FILES_SAID is how many files a tooltip names; the rest are counted
export const MAX_FILES_SAID = 6;

// gravest is the collision a mark speaks for: one on the same lines before one on the same files, the server's order otherwise
export function gravest(collisions: Collision[] | undefined): Collision | undefined {
  return collisions?.find((collision) => collision.lines) ?? collisions?.[0];
}
