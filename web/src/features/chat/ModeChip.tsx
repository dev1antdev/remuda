// ModeChip is Claude Code's permission mode as its footer names it — accept edits, plan, auto, manual, bypass — off the
// chat's one read of the pane's screen (useScreenRead), beside the composer's tools; a tap sends shift+tab, the key that
// cycles it and that a phone has no way to send. Nothing is drawn while no footer is on screen: a dialog over it, another agent.

import { useState } from "react";
import { act } from "@/app/act";
import { sendKeys } from "@/features/roster/api";
import { t } from "@/lib/i18n";
import { Icon, Tip } from "@/ui";
import { MODE_ICON, isPermissionMode, modeTone, modeWords } from "./mode";

export type ScreenMode = { mode?: string; reread: () => void }; // the footer's mode as last read, and a read again

export function ModeChip({ machineId, paneId, screen }: { machineId: string; paneId: string; screen: ScreenMode }) {
  const [sending, setSending] = useState(false);
  if (!isPermissionMode(screen.mode)) return null;
  const mode = screen.mode;
  const cycle = async () => {
    setSending(true);
    try {
      await act(sendKeys(machineId, paneId, ["shift+tab"]));
    } finally {
      setSending(false);
      setTimeout(screen.reread, 600); // the footer after the key: the next mode
    }
  };
  const words = modeWords(mode);
  return (
    <Tip text={t("Permission mode: {mode} · tap to cycle (shift+tab)", { mode: words })}>
      <button
        type="button"
        className={`btn sm mode-chip${modeTone(mode) === "amber" ? " is-bypass" : ""}`}
        aria-label={t("Permission mode: {mode}", { mode: words })}
        disabled={sending}
        onClick={() => void cycle()}
      >
        <Icon name={MODE_ICON[mode]} size={12} /> {words}
      </button>
    </Tip>
  );
}
