// The words of the Fleet setup table, and what it offers: a row's label and its explanation, a value as a person reads
// it, the updates that bring a machine in line and what is left to do by hand. Pure, so the table's every rule is tested.

import { t, tn } from "@/lib/i18n";
import { ago } from "@/lib/time";
import type { FleetAction, FleetCell, FleetRow, FleetSection, FleetTable } from "./api";

export const AGENT_TITLES: Record<string, string> = { claude: "Claude Code", codex: "Codex", gemini: "Gemini CLI", kimi: "Kimi Code" };
const agentTitle = (agent = "") => AGENT_TITLES[agent] ?? agent;

// the global instruction file each agent reads in every session
export const INSTRUCTION_FILES: Record<string, string> = { claude: "~/.claude/CLAUDE.md", codex: "~/.codex/AGENTS.md", gemini: "~/.gemini/GEMINI.md" };

export function sectionTitle(section: FleetSection): string {
  switch (section.id) {
    case "agents":
      return t("Agents");
    case "herdr":
      return "herdr";
    case "remuda":
      return "remuda";
    case "claude":
      return t("Claude Code's settings");
    case "instructions":
      return t("Global instructions");
  }
  return section.name ?? "";
}

export function rowLabel(row: FleetRow): string {
  const agent = agentTitle(row.arg);
  switch (row.id) {
    case "agent.version":
      return agent;
    case "agent.via":
      return t("{agent}: installed with", { agent });
    case "herdr.version":
      return t("herdr installed");
    case "herdr.server":
      return t("herdr's running server");
    case "herdr.hooks":
      return t("{agent} hooks", { agent });
    case "herdr.plugin":
      return t("Plugin {id}", { id: row.arg ?? "" });
    case "remuda.version":
      return "remuda";
    case "code.version":
      return "code-server";
    case "mcp":
      return t("Tasks for {agent} (MCP)", { agent });
    case "statusline":
      return t("Plan limits in the status line");
    case "claude.model":
      return t("Default model");
    case "claude.fast":
      return t("Fast mode");
    case "instructions":
      return INSTRUCTION_FILES[row.arg ?? ""] ?? row.arg ?? "";
    case "repo.branch":
      return t("Branch");
    case "repo.behind":
      return t("Behind its upstream");
  }
  return row.arg ?? row.id;
}

// rowExplain is the row's tooltip: what the fact is and where it is read
export function rowExplain(row: FleetRow): string {
  const agent = agentTitle(row.arg);
  switch (row.id) {
    case "agent.version":
      return t("The {agent} version on each machine, as the vitals last read it.", { agent });
    case "agent.via":
      return t("How {agent} was installed there — Homebrew, npm or its own installer — which decides how it is updated.", { agent });
    case "herdr.version":
      return t("The herdr installed on each machine.");
    case "herdr.server":
      return t("The herdr server running on each machine: one still on an older version than the herdr installed is restarted onto it.");
    case "herdr.hooks":
      return t("herdr's {agent} hooks, which tell herdr, and so remuda, what the agent is doing: current, outdated or missing.", { agent });
    case "herdr.plugin":
      return t("Whether herdr's plugin {id} is installed there, and where from.", { id: row.arg ?? "" });
    case "remuda.version":
      return t("The remuda on each machine: it keeps the agents' door to your tasks there and relays its terminals.");
    case "code.version":
      return t("The code-server the Code mode opens on each machine.");
    case "mcp":
      return t("Whether remuda's MCP server — your task list for the agents — is registered in {agent}'s own config there.", { agent });
    case "statusline":
      return t("Whether Claude Code's status line goes through remuda there, which keeps its plan limits for the meter.");
    case "claude.model":
      return t("The default model Claude Code's user settings keep there, in ~/.claude/settings.json.");
    case "claude.fast":
      return t("Whether fast mode is saved on in Claude Code's user settings there.");
    case "instructions":
      return t("{file}: what {agent} reads in every session there, compared by a hash of its content.", { file: rowLabel(row), agent });
    case "repo.branch":
      return t("The branch checked out in the repository's main checkout on each machine.");
    case "repo.behind":
      return t("Commits on the checkout's upstream it does not have yet, as of the last fetch there.");
    case "repo.file":
      return t("{file} in the repository's main checkout, compared by a hash of its content.", { file: row.arg ?? "" });
  }
  return "";
}

// valueWords is a cell as the table shows it
export function valueWords(row: FleetRow, cell: FleetCell): string {
  const value = cell.value;
  switch (row.kind) {
    case "version":
      return value || (row.id === "herdr.server" ? t("not running") : t("not installed"));
    case "via":
      return value === "other" ? t("another way") : value;
    case "flag":
      return value === "yes" ? (row.id === "claude.fast" ? t("on") : t("yes")) : row.id === "claude.fast" ? t("off") : t("no");
    case "hook":
      return value === "current" ? t("current") : value === "outdated" ? t("outdated") : value === "missing" ? t("missing") : value;
    case "plugin":
      return !value ? t("not installed") : value.startsWith("github:") ? value.slice("github:".length) : t("local");
    case "model":
      return value === "default" ? t("Claude Code's default") : value;
    case "hash":
      return !value ? t("absent") : value === "present" ? t("present") : value.slice(0, 7);
    case "behind":
      return value === "0" ? t("up to date") : tn(Number(value), "{n} commit behind", "{n} commits behind");
  }
  return value || "—";
}

// cellTip is what a cell says on a closer look: a file's hash and its last change, a checkout's upstream and its last fetch
export function cellTip(row: FleetRow, cell: FleetCell, now = Date.now()): string {
  const when = cell.at ? ago(cell.at * 1000, now) : "";
  if (row.kind === "hash" && cell.value && cell.value !== "present") return when ? t("sha256 {hash}, changed {when}", { hash: cell.value, when }) : cell.value;
  if (row.kind === "behind") return when ? t("against {upstream}, fetched {when}", { upstream: cell.note ?? "", when }) : t("against {upstream}, never fetched there", { upstream: cell.note ?? "" });
  if (row.kind === "plugin" && cell.value) return cell.value;
  return "";
}

export const differs = (row: FleetRow): boolean => row.state === "odd" || row.state === "split";

// shownSections is the table's sections with every row, or with the rows that differ alone; a section left empty goes
export function shownSections(table: FleetTable, all: boolean): FleetSection[] {
  return table.sections.map((section) => ({ ...section, rows: all ? section.rows : section.rows.filter(differs) })).filter((section) => section.rows.length > 0);
}

export function summaryWords(table: FleetTable): string {
  const rows = table.sections.flatMap((section) => section.rows);
  const odd = rows.filter(differs).length;
  const read = table.machines.filter((m) => !m.away && !m.error).length;
  if (read < 2) return t("Connect a second machine to compare how they are set up.");
  if (odd === 0) return tn(rows.length, "Every machine is set up alike: {n} fact compared.", "Every machine is set up alike: {n} facts compared.");
  return tn(odd, "{n} of {total} facts differs between the machines.", "{n} of {total} facts differ between the machines.", { total: rows.length });
}

// Bring is one update remuda already has, on one machine, and the rows it brings in line
export type Bring = { machine: string; action: FleetAction; rows: string[] };

// bringInLine is every update the table offers, a machine's in the columns' order, each update once however many rows it fixes
export function bringInLine(table: FleetTable): Bring[] {
  const out: Bring[] = [];
  for (const column of table.machines)
    for (const section of table.sections)
      for (const row of section.rows) {
        const action = row.cells[column.id]?.action;
        if (!action) continue;
        const label = section.id === "repo" ? `${section.name} · ${rowLabel(row)}` : rowLabel(row);
        const known = out.find((b) => b.machine === column.id && b.action.what === action.what);
        if (known) known.rows.push(label);
        else out.push({ machine: column.id, action, rows: [label] });
      }
  return out;
}

// ByHand is a row that differs with no update to bring it in line: what to do, said for the machines it names
export type ByHand = { key: string; title: string; words: string };

// byHand is every row with machines no update brings in line (the server's hand); name says a machine as the page shows it
export function byHand(table: FleetTable, name: (machine: string) => string, now = Date.now()): ByHand[] {
  const out: ByHand[] = [];
  for (const section of table.sections)
    for (const row of section.rows) {
      const hand = row.hand ?? [];
      if (!differs(row) || hand.length === 0) continue;
      const others = table.machines.filter((m) => row.cells[m.id] && !hand.includes(m.id)).map((m) => m.id);
      const title = section.id === "repo" || section.id === "claude" ? `${sectionTitle(section)} · ${rowLabel(row)}` : rowLabel(row);
      const missing = hand.every((id) => !row.cells[id]?.value);
      out.push({ key: `${section.id}|${section.name ?? ""}|${row.id}|${row.arg ?? ""}`, title, words: handWords(row, hand.map(name), others.map(name), missing, newestOf(row, table, name, now)) });
    }
  return out;
}

// newestOf names the machine whose copy of a file changed last, and when, for a row of files that differ
function newestOf(row: FleetRow, table: FleetTable, name: (machine: string) => string, now: number): string {
  if (row.kind !== "hash") return "";
  const newest = table.machines.map((m) => ({ id: m.id, at: row.cells[m.id]?.at ?? 0 })).sort((a, b) => b.at - a.at)[0];
  return newest && newest.at ? t("the copy changed last is {machine}'s, {when}", { machine: name(newest.id), when: ago(newest.at * 1000, now) }) : "";
}

// handWords says what to do by hand on the machines named, missing when none of them has the thing at all: remuda never
// writes the agents' configs beyond its own updates
export function handWords(row: FleetRow, hand: string[], others: string[], missing: boolean, newest = ""): string {
  const machines = hand.join(", ");
  const agent = agentTitle(row.arg);
  const tail = newest ? ` (${newest})` : "";
  switch (row.id) {
    case "agent.version":
      return missing
        ? t("Install {agent} on {machines} with its own installer, then run remuda setup there for herdr's hooks — or leave it, if it is not wanted there.", { agent, machines })
        : t("Update {agent} on {machines} by hand: remuda has no update for it there.", { agent, machines });
    case "agent.via":
      return t("{agent} is installed another way on {machines}; remuda updates each the way it was installed. To match, reinstall it there by hand.", { agent, machines });
    case "herdr.version":
    case "herdr.server":
      return t("Bring herdr on {machines} in line by hand: remuda setup there installs it and starts its server.", { machines });
    case "herdr.hooks":
      return t("Run herdr integration install {agent} on {machines}.", { agent: row.arg ?? "", machines });
    case "herdr.plugin":
      return missing ? t("Install the plugin on {machines} as it is on {others}, or remove it where it is not wanted.", { machines, others: others.join(", ") }) : t("The plugin comes from another place on {machines}: install it from the same one everywhere.", { machines });
    case "remuda.version":
      return t("Install the same remuda on {machines}: the installer's one line there, or Updates once a newer release is out.", { machines });
    case "code.version":
      return t("Run remuda code install on {machines}.", { machines });
    case "mcp":
      return t("Run remuda mcp install on {machines}.", { machines });
    case "statusline":
      return t("Run remuda statusline install on {machines}, or remuda statusline uninstall where it is not wanted.", { machines });
    case "claude.model":
      return t("Set Claude Code's default model on {machines} by hand: /model in Claude Code there saves the pick, or edit ~/.claude/settings.json. remuda never writes the agents' settings.", { machines });
    case "claude.fast":
      return t("Fast mode is set otherwise on {machines}: switch it in Claude Code there, or in ~/.claude/settings.json. remuda never writes the agents' settings.", { machines });
    case "instructions":
      return t("{file} differs on {machines}{tail}: copy the one you want to the others by hand. remuda never writes the agents' configs.", { file: rowLabel(row), machines, tail });
    case "repo.branch":
      return t("Another branch is checked out on {machines}: switch it there, unless that is on purpose.", { machines });
    case "repo.behind":
      return t("Pull on {machines}: the checkout there is behind its upstream as of its last fetch.", { machines });
    case "repo.file":
      return t("{file} differs on {machines}{tail}: commit it where it changed and pull it on the others.", { file: row.arg ?? "", machines, tail });
  }
  return t("Differs on {machines}.", { machines });
}
