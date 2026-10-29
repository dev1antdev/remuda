// Claude Code's permission mode as its footer names it, read by the server off the pane's screen: the words the chip
// shows, the icon it wears, and which one deserves the amber. The pure part of ModeChip.

import { t } from "@/lib/i18n";
import type { IconName } from "@/ui/icons";

export type PermissionMode = "default" | "acceptEdits" | "plan" | "auto" | "bypass";

const WORDS: Record<PermissionMode, string> = {
  default: "manual mode",
  acceptEdits: "accept edits",
  plan: "plan mode",
  auto: "auto mode",
  bypass: "bypass permissions",
};

export const MODE_ICON: Record<PermissionMode, IconName> = {
  default: "keyboard",
  acceptEdits: "edit",
  plan: "list",
  auto: "sparkles",
  bypass: "alert-triangle",
};

// isPermissionMode keeps only the modes the server names
export const isPermissionMode = (value: unknown): value is PermissionMode => typeof value === "string" && value in WORDS;

// modeWords is what the chip says for a mode, in the page's language
export const modeWords = (mode: PermissionMode): string => t(WORDS[mode]);

// bypass alone wears the amber: it is the one mode that asks nothing
export const modeTone = (mode: PermissionMode): "amber" | null => (mode === "bypass" ? "amber" : null);
