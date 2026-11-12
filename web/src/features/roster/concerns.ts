// What a machine's vitals say wants a look: an agent, herdr or remuda behind the newest, a nearly full disk, Claude
// Code's sign-in running out, something set to a model being retired.

import { newer } from "@/lib/version";
import { VENDOR_NAME } from "@/ui/brand";
import type { Health, Latest, StoppedChat } from "./api";
import { bodyConcerns } from "./body";
import { stoppedNames, stoppedWords } from "./stopped";
import { signinConcern } from "./signin";
import { retiringConcerns, type Retirement, type RetiringHit } from "@/features/usage/retired";
import { t } from "@/lib/i18n";

export type Concern = { key: string; text: string; bad?: boolean; update?: string };

// isOut says a newer version of something is out: "Claude Code 2.2.0 is out (2.1.0 here)"
const isOut = (what: string, version: string, have: string): string => t("{what} {version} is out ({have} here)", { what, version, have });

// concerns lists what wants a look, each with the update that answers it; a viewer link's page reads the same words with
// no update, since every one of them is a change the server refuses it
export function concerns(h: Health | undefined, latest: Latest | undefined, serverVersion: string | undefined, remote: boolean, stopped: StoppedChat[] = [], viewOnly = false, more: More = {}): Concern[] {
  const out = found(h, latest, serverVersion, remote, stopped, more);
  return viewOnly ? out.map(({ update: _offered, ...words }) => words) : out;
}

// More is what the machine adds to its vitals: its name for the words, what is set there to a model being retired
// with the roster's table of retirements its hits name, and the clock the sign-in is weighed by
export type More = { machine?: string; retiring?: RetiringHit[]; retirements?: Retirement[]; now?: number };

function found(h: Health | undefined, latest: Latest | undefined, serverVersion: string | undefined, remote: boolean, stopped: StoppedChat[], more: More = {}): Concern[] {
  const out: Concern[] = [];
  const now = more.now ?? Date.now();
  if (stopped.length) out.push({ key: "stopped", text: `${stoppedWords(stopped.length)}: ${stoppedNames(stopped)}`, update: "recover" });
  const signin = signinConcern(h?.claude_login, more.machine ?? "", now);
  if (signin) out.push({ key: "signin", ...signin, update: "signin" });
  out.push(...retiringConcerns(more.retiring, more.retirements ?? [], now));
  if (!h) return out;
  if (h.claude && latest?.claude && newer(h.claude, latest.claude)) out.push({ key: "claude", text: isOut("Claude Code", latest.claude, h.claude), update: "claude" });
  if (h.codex && latest?.codex && newer(h.codex, latest.codex)) out.push({ key: "codex", text: isOut("Codex", latest.codex, h.codex), update: "codex" });
  if (h.gemini && latest?.gemini && newer(h.gemini, latest.gemini)) out.push({ key: "gemini", text: isOut("Gemini CLI", latest.gemini, h.gemini), update: "gemini" });
  if (h.kimi && latest?.kimi && newer(h.kimi, latest.kimi)) out.push({ key: "kimi", text: isOut("Kimi Code", latest.kimi, h.kimi), update: "kimi" });
  if (h.herdr && latest?.herdr && newer(h.herdr, latest.herdr)) out.push({ key: "herdr", text: isOut("herdr", latest.herdr, h.herdr), update: "herdr" });
  if (h.herdr && h.herdr_server && h.herdr_server !== h.herdr) out.push({ key: "herdr-server", text: t("herdr's server still runs {server}; {installed} is installed", { server: h.herdr_server, installed: h.herdr }), update: "restart-herdr" });
  if (h.code_server && latest?.code && newer(h.code_server, latest.code)) out.push({ key: "code", text: t("code-server {version} is what remuda installs now ({have} here)", { version: latest.code, have: h.code_server }), update: "code" });
  if (h.outdated_hooks?.length) out.push({ key: "hooks", text: t("herdr's hooks for {agents} are outdated", { agents: hookNames(h.outdated_hooks) }), update: "hooks" });
  if (remote && serverVersion && h.remuda && h.remuda !== serverVersion) out.push({ key: "ui", text: t("remuda there is {there}; this one is {here}", { there: h.remuda, here: serverVersion }), update: "remuda" });
  if (remote && serverVersion && !h.remuda) out.push({ key: "ui", text: t("no remuda deployed there — search and usage fall back to slower paths"), update: "remuda" });
  const have = remote ? h.remuda : serverVersion;
  if (have && latest?.remuda && newer(have, latest.remuda)) out.push({ key: "release", text: isOut("remuda", latest.remuda, have), update: "remuda-release" });
  if (h.disk_used_pct >= 90 || (h.disk_free_gb > 0 && h.disk_free_gb < 10)) out.push({ key: "disk", text: t("Disk {pct}% full, {gb} GB free", { pct: h.disk_used_pct, gb: h.disk_free_gb.toFixed(0) }), bad: h.disk_used_pct >= 95 });
  bodyConcerns(h).forEach((c, i) => out.push({ key: `body-${i}`, ...c }));
  if (h.error && !h.claude && !h.herdr) out.push({ key: "err", text: h.error, bad: true });
  return out;
}

// bodyOrDisk is a concern about the host itself: its disk, its body, a probe that failed
const bodyOrDisk = (key: string) => key === "disk" || key === "err" || key.startsWith("body-");

// hostConcerns are what the Server place lists: the host's own release and what bodyOrDisk keeps — a herdr, its hooks or
// an agent still installed there serve no workspace, so their versions and Claude Code's sign-in are none of a server's business
export function hostConcerns(h: Health | undefined, latest: Latest | undefined, serverVersion: string | undefined, viewOnly = false): Concern[] {
  return concerns(h, latest, serverVersion, false, [], viewOnly).filter((c) => c.key === "release" || bodyOrDisk(c.key));
}

// serverTone is the Server place's icon on the rail: red for anything bad in the host's body or disk, amber for a look,
// none when all is well; the versions are the Updates' business, not the icon's
export function serverTone(h: Health | undefined): "bad" | "warn" | "" {
  const list = found(h, undefined, undefined, false, []).filter((c) => bodyOrDisk(c.key));
  return list.some((c) => c.bad) ? "bad" : list.length ? "warn" : "";
}

// hookNames says which agents' herdr hooks are behind, by the agents' names
function hookNames(hooks: { agent: string }[]): string {
  const names = hooks.map((h) => VENDOR_NAME[h.agent] ?? h.agent);
  const last = names.pop() ?? "";
  return names.length ? t("{names} and {last}", { names: names.join(", "), last }) : last;
}

// updateLabel is the button's word for what an update does
export const updateLabel = (what: string) =>
  what === "remuda" ? t("Deploy") : what === "remuda-release" ? t("Download & install") : what === "recover" ? t("Resume") : what === "signin" ? t("Open a shell there") : what.startsWith("restart-") ? t("Restart") : t("Update");
