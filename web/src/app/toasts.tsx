// The toasts in the corner: offline, a newer server, a release out, Tailscale's state, the last notice.

import { useEffect } from "react";
import { usePref } from "@/lib/prefs";
import { newer } from "@/lib/version";
import { useRoster } from "@/features/roster/useRoster";
import type { ToastItem } from "@/ui";
import { actions, app } from "./store";

export function useToasts(): ToastItem[] {
  const { machines, latest, tailscale, version, offline, stale } = useRoster();
  const notice = app.use((s) => s.notice);
  const [releaseSeen, setReleaseSeen] = usePref("release.seen", "");
  useEffect(() => {
    if (!notice || notice.error) return;
    const t = setTimeout(actions.clearNotice, notice.brief ? 2000 : 6000);
    return () => clearTimeout(t);
  }, [notice]);

  const toasts: ToastItem[] = [];
  if (offline) toasts.push({ id: "offline", tone: "warn", text: "Reconnecting to remuda…", busy: true });
  if (stale && !offline)
    toasts.push({
      id: "stale",
      tone: "info",
      text: `remuda ${stale} is on the server; this page runs ${__REMUDA_VERSION__}.`,
      action: (
        <button type="button" className="btn sm primary" onClick={() => location.reload()}>
          Reload
        </button>
      ),
    });
  const release = latest?.remuda;
  const local = machines.find((m) => m.id === "local");
  if (release && version && newer(version, release) && releaseSeen !== release && !offline)
    toasts.push({
      id: "release",
      tone: "info",
      text: `remuda ${release} is out; this one is ${version}.`,
      action: local && (
        <button type="button" className="btn sm primary" onClick={() => actions.openPanel("release")}>
          Download & install
        </button>
      ),
      onDismiss: () => setReleaseSeen(release),
    });
  if (notice) toasts.push({ id: "notice", tone: notice.error ? "error" : "success", text: notice.text, onDismiss: actions.clearNotice });
  if (tailscale?.state === "needs_login" && tailscale.login_url)
    toasts.push({
      id: "tailscale",
      tone: "info",
      text: "Tailscale: this remuda is not on your tailnet yet.",
      action: (
        <a className="btn sm primary" href={tailscale.login_url} target="_blank" rel="noreferrer">
          Log in
        </a>
      ),
    });
  if (tailscale?.state === "error") toasts.push({ id: "tailscale", tone: "error", text: `Tailscale: ${tailscale.error}` });
  return toasts;
}
