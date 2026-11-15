// What stands in the person's slot of a conversation without being theirs: a background task reporting back, the note
// of an interruption, the summary a compaction goes on from. Claude Code writes each as a user record; the chat draws
// it as a quiet line with the record folded under it, never as a message the person sent.

import { t } from "@/lib/i18n";
import type { IconName } from "@/ui/icons";
import { COMPACTION_PREFIX } from "./timeline";

export type NoteKind = "task" | "interrupted" | "compacted";
export type Note = { kind: NoteKind; icon: IconName; words: string; failed: boolean };

const TASK_PREFIX = "<task-notification>";
const INTERRUPTED_PREFIX = "[Request interrupted";

// noteKind tells which note a user turn's text is, or null for words of the person's own
export function noteKind(text: string): NoteKind | null {
  const said = text.trimStart();
  if (said.startsWith(TASK_PREFIX)) return "task";
  if (said.startsWith(INTERRUPTED_PREFIX)) return "interrupted";
  if (said.startsWith(COMPACTION_PREFIX)) return "compacted";
  return null;
}

// isNote says a user turn's text is the harness's, not the person's
export const isNote = (text: string): boolean => noteKind(text) !== null;

// tagged is what a task's note holds between one of its tags, "" without it; the note is a few lines, so the search
// for the closing tag is one pass
export function tagged(text: string, tag: string): string {
  const from = text.indexOf(`<${tag}>`);
  if (from < 0) return "";
  const to = text.indexOf(`</${tag}>`, from);
  return to < 0 ? "" : text.slice(from + tag.length + 2, to).trim();
}

// noteOf is the note a user turn's text is, in the page's words: what a background task came to, that the person
// stopped the agent, that the conversation was compacted; null for words of the person's own
export function noteOf(text: string): Note | null {
  const kind = noteKind(text);
  if (kind === "task") {
    const status = tagged(text, "status");
    return { kind, icon: "activity", words: tagged(text, "summary") || t("A background task reported back"), failed: status === "failed" || status === "killed" };
  }
  if (kind === "interrupted") return { kind, icon: "square", words: text.includes("for tool use") ? t("You stopped the agent at a tool call") : t("You stopped the agent"), failed: false };
  if (kind === "compacted") return { kind, icon: "archive", words: t("Compacted: the conversation goes on from this summary of what came before"), failed: false };
  return null;
}
