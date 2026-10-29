// ModeChip is Claude Code's permission mode as its footer names it — accept edits, plan, auto, manual, bypass — read off
// the pane's screen while the lens is on, beside the composer's tools; a tap sends shift+tab, the key that cycles it and
// that a phone has no way to send. Nothing is drawn while no footer is on screen: a dialog over it, another agent.

import { useState } from "react";
import { act } from "@/app/act";
import { sendKeys } from "@/features/roster/api";
import { t } from "@/lib/i18n";
import { useLiveConnected } from "@/lib/live";
import { useQuery } from "@/lib/query";
import { Icon, Tip } from "@/ui";
import { fetchMode } from "./api";
import { MODE_ICON, isPermissionMode, modeTone, modeWords } from "./mode";

export function ModeChip({ machineId, paneId, active }: { machineId: string; paneId: string; active: boolean }) {
  const connected = useLiveConnected();
  const { data: read, reload } = useQuery(() => fetchMode(machineId, paneId), [machineId, paneId], { every: connected ? 15_000 : 10_000, enabled: active, keep: true });
  const [sending, setSending] = useState(false);
  if (!isPermissionMode(read)) return null;
  const mode = read;
  const cycle = async () => {
    setSending(true);
    try {
      await act(sendKeys(machineId, paneId, ["shift+tab"]));
    } finally {
      setSending(false);
      setTimeout(reload, 600); // the footer after the key: the next mode
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
