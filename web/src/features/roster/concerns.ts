// What a machine's vitals say wants a look: an agent, herdr or remuda behind the newest, a nearly full disk.

import { newer } from "@/lib/version";
import { VENDOR_NAME } from "@/ui/brand";
import type { Health, Latest } from "./api";

export type Concern = { key: string; text: string; bad?: boolean; update?: string };

export function concerns(h: Health | undefined, latest: Latest | undefined, serverVersion: string | undefined, remote: boolean): Concern[] {
  if (!h) return [];
  const out: Concern[] = [];
  if (h.claude && latest?.claude && newer(h.claude, latest.claude)) out.push({ key: "claude", text: `Claude Code ${latest.claude} is out (${h.claude} here)`, update: "claude" });
  if (h.codex && latest?.codex && newer(h.codex, latest.codex)) out.push({ key: "codex", text: `Codex ${latest.codex} is out (${h.codex} here)`, update: "codex" });
  if (h.gemini && latest?.gemini && newer(h.gemini, latest.gemini)) out.push({ key: "gemini", text: `Gemini CLI ${latest.gemini} is out (${h.gemini} here)`, update: "gemini" });
  if (h.herdr && latest?.herdr && newer(h.herdr, latest.herdr)) out.push({ key: "herdr", text: `herdr ${latest.herdr} is out (${h.herdr} here)`, update: "herdr" });
  if (h.herdr && h.herdr_server && h.herdr_server !== h.herdr) out.push({ key: "herdr-server", text: `herdr's server still runs ${h.herdr_server}; ${h.herdr} is installed`, update: "restart-herdr" });
  if (h.outdated_hooks?.length) out.push({ key: "hooks", text: `herdr's hooks for ${hookNames(h.outdated_hooks)} are outdated`, update: "hooks" });
  if (remote && serverVersion && h.remuda && h.remuda !== serverVersion) out.push({ key: "ui", text: `remuda there is ${h.remuda}; this one is ${serverVersion}`, update: "remuda" });
  if (remote && serverVersion && !h.remuda) out.push({ key: "ui", text: "no remuda deployed there — search and usage fall back to slower paths", update: "remuda" });
  const have = remote ? h.remuda : serverVersion;
  if (have && latest?.remuda && newer(have, latest.remuda)) out.push({ key: "release", text: `remuda ${latest.remuda} is out (${have} here)`, update: "remuda-release" });
  if (h.disk_used_pct >= 90 || (h.disk_free_gb > 0 && h.disk_free_gb < 10)) out.push({ key: "disk", text: `Disk ${h.disk_used_pct}% full, ${h.disk_free_gb.toFixed(0)} GB free`, bad: h.disk_used_pct >= 95 });
  if (h.error && !h.claude && !h.herdr) out.push({ key: "err", text: h.error, bad: true });
  return out;
}

// hookNames says which agents' herdr hooks are behind, by the agents' names
function hookNames(hooks: { agent: string }[]): string {
  const names = hooks.map((h) => VENDOR_NAME[h.agent] ?? h.agent);
  const last = names.pop() ?? "";
  return names.length ? `${names.join(", ")} and ${last}` : last;
}

// updateLabel is the button's word for what an update does
export const updateLabel = (what: string) => (what === "remuda" ? "Deploy" : what === "remuda-release" ? "Download & install" : what.startsWith("restart-") ? "Restart" : "Update");
