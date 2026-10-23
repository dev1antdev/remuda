// A conversation as a Markdown file, headed by where it ran: the turns as the lens reads them — text as it is,
// thinking and tool calls folded behind <details>. conversationMarkdown and exportFileName are pure, tested;
// exportConversation fetches the session and hands the file to the browser.

import { saveText } from "@/lib/host";
import { fmtDate } from "@/lib/time";
import { VENDOR_NAME } from "@/ui/brand";
import { fetchChat, type Part, type Turn } from "./api";
import { toolLabel } from "./footprint";

export type ExportMeta = {
  agent: string; // the vendor key, "" for a transcript on its own
  workspace: string;
  machine: string; // the machine's label
  sessionId?: string;
  when: Date; // when the export was made
};

export function conversationMarkdown(turns: Turn[], meta: ExportMeta): string {
  const who = meta.agent ? (VENDOR_NAME[meta.agent] ?? meta.agent) : "Agent";
  const out: string[] = [`# ${meta.workspace} · ${who} on ${meta.machine}`, ""];
  const made = [`Exported from remuda on ${fmtDate(meta.when)}`, meta.sessionId ? `session ${meta.sessionId}` : "", `${turns.length} turn${turns.length === 1 ? "" : "s"}`].filter(Boolean).join(" · ");
  out.push(made, "", "---", "");
  for (const turn of turns) {
    const at = turn.ts ? ` · ${new Date(turn.ts).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}` : "";
    out.push(`**${turn.role === "user" ? "You" : who}**${at}`, "");
    for (const part of turn.parts) out.push(...partMarkdown(part), "");
  }
  return out.join("\n").replace(/\n{3,}/g, "\n\n").trimEnd() + "\n";
}

function partMarkdown(part: Part): string[] {
  if (part.type === "text") return [part.text ?? ""];
  if (part.type === "thinking") return folded("Thinking", part.text ?? "", false);
  const { name, hint } = toolLabel(part);
  const title = [name, hint].filter(Boolean).join(" · ") + (part.is_error ? " · error" : "");
  return folded(title, part.output ?? "", true);
}

// folded is a <details> block, its body fenced when it is a tool's output; the fence outruns any backticks inside
function folded(summary: string, body: string, fence: boolean): string[] {
  const text = body.trimEnd();
  if (!text) return [`<details><summary>${escapeHtml(summary)}</summary></details>`];
  const inner = fence ? [ticks(text), text, ticks(text)] : [text];
  return [`<details><summary>${escapeHtml(summary)}</summary>`, "", ...inner, "", "</details>"];
}

const ticks = (text: string) => "`".repeat(Math.max(3, (text.match(/`+/g) ?? []).reduce((n, run) => Math.max(n, run.length), 0) + 1));

const escapeHtml = (text: string) => text.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");

// exportFileName is "<workspace>-<yyyy-mm-dd>-<session's first 8>.md", the workspace cut to a file-safe stem
export function exportFileName(meta: ExportMeta): string {
  const stem = meta.workspace.replace(/[^\p{L}\p{N}._-]+/gu, "-").replace(/^-+|-+$/g, "").slice(0, 40) || "conversation";
  const day = meta.when.toISOString().slice(0, 10);
  return `${stem}-${day}${meta.sessionId ? `-${meta.sessionId.slice(0, 8)}` : ""}.md`;
}

// exportConversation reads the session whole — the pane's live one, an older one of its folder, or a transcript on its own — and saves it
export async function exportConversation(machineId: string, paneId: string, sessionId: string | null, dir: string, meta: Omit<ExportMeta, "sessionId" | "when">): Promise<void> {
  const res = await fetchChat(machineId, paneId, sessionId, null, dir);
  if (!res) return;
  const full: ExportMeta = { ...meta, sessionId: res.body.session_id, when: new Date() };
  saveText(exportFileName(full), conversationMarkdown(res.body.turns, full), "text/markdown");
}
