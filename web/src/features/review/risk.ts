// A review's risk in words: the score the server gave a workspace's uncommitted work out of 100, its level and each
// reason — the change's size, code with no test beside it, a migration, a lockfile, CI config, auth-looking paths, the
// secret check's findings, its pull request's checks, a collision with another checkout. The pure part, tested; the
// review queue draws the chip.

import type { Risk, RiskReason } from "@/features/roster/api";
import { t, tn } from "@/lib/i18n";

// pathsWords names the first paths a reason carries and counts the rest
export function pathsWords(paths: string[] | undefined, count = paths?.length ?? 0): string {
  const named = paths ?? [];
  const rest = count - named.length;
  return rest > 0 ? t("{paths} and {n} more", { paths: named.join(", "), n: rest }) : named.join(", ");
}

// BIG_FROM is the points a big change weighs, the server's pointsBig: less is a sizeable one
const BIG_FROM = 25;

// reasonWords is one reason as the tooltip says it
export function reasonWords(reason: RiskReason): string {
  const count = reason.count ?? 0;
  const paths = pathsWords(reason.paths, count);
  switch (reason.kind) {
    case "size": {
      const files = tn(count, "{n} file", "{n} files");
      return reason.points >= BIG_FROM ? t("A big change: about {lines} lines in {files}", { lines: reason.lines ?? 0, files }) : t("A sizeable change: about {lines} lines in {files}", { lines: reason.lines ?? 0, files });
    }
    case "untested":
      return tn(count, "{n} source file changed with no test beside it: {paths}", "{n} source files changed with no test beside them: {paths}", { paths });
    case "migration":
      return t("A migration or schema: {paths}", { paths });
    case "lockfile":
      return t("The dependencies change: {paths}", { paths });
    case "ci":
      return t("What CI runs changes: {paths}", { paths });
    case "auth":
      return t("Sign-in, permissions or secrets code: {paths}", { paths });
    case "secrets":
      return tn(count, "The secret check found {n} possible secret", "The secret check found {n} possible secrets");
    case "checks":
      return tn(count, "{n} check fails on its pull request", "{n} checks fail on its pull request");
    case "conflicts":
      return t("Its pull request has conflicts");
    case "changes":
      return t("A reviewer asked for changes on its pull request");
    case "collision":
      return reason.same ? t("Changes the same lines as {peer}", { peer: reason.with ?? "" }) : t("Changes the same files as {peer}", { peer: reason.with ?? "" });
  }
  return reason.kind;
}

// levelWords is a level in words
export function levelWords(level: Risk["level"]): string {
  return level === "high" ? t("high risk") : level === "medium" ? t("medium risk") : t("low risk");
}

// riskChip is the chip's text: the score alone, the level its colour
export function riskChip(risk: Risk): string {
  return t("risk {score}", { score: risk.score });
}

// riskHead is the score and its level, the tooltip's first line and all of it while the screen-sharing mode hides the folder
export function riskHead(risk: Risk): string {
  return t("Risk {score} of 100: {level}", { score: risk.score, level: levelWords(risk.level) });
}

// riskTip is the chip's tooltip: the score and its level, then a line per reason with its points
export function riskTip(risk: Risk): string {
  const head = riskHead(risk);
  if (risk.reasons.length === 0) return `${head}\n${t("A small change with nothing that asks for a second look.")}`;
  return [head, ...risk.reasons.map((reason) => `· ${reasonWords(reason)} (+${reason.points})`)].join("\n");
}

// scoreOf is a risk's score for the queue's order, 0 for none read
export function scoreOf(risk: Risk | undefined): number {
  return risk?.score ?? 0;
}
