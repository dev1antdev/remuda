// The toasts in the corner: offline, a newer server, a release out, what happened while the page was away, Tailscale's
// state, the last notice.

import { useEffect } from "react";
import { usePref } from "@/lib/prefs";
import { newer } from "@/lib/version";
import { episodeKey, remember, stoppedWords } from "@/features/roster/stopped";
import { resumeStopped } from "@/features/updates/recover";
import { useRoster } from "@/features/roster/useRoster";
import type { ToastItem } from "@/ui";
import { actions, app } from "./store";
import { t } from "@/lib/i18n";

export function useToasts(): ToastItem[] {
  const { machines, latest, tailscale, version, offline, stale } = useRoster();
  const notice = app.use((s) => s.notice);
  const away = app.use((s) => s.away);
  const [releaseSeen, setReleaseSeen] = usePref("release.seen", "");
  const [recoverSeen, setRecoverSeen] = usePref<string[]>("recover.seen", []);
  useEffect(() => {
    if (!notice || notice.error) return;
    const t = setTimeout(actions.clearNotice, notice.brief ? 2000 : 6000);
    return () => clearTimeout(t);
  }, [notice]);

  const toasts: ToastItem[] = [];
  if (offline) toasts.push({ id: "offline", tone: "warn", text: t("Reconnecting to remuda…"), busy: true });
  if (stale && !offline)
    toasts.push({
      id: "stale",
      tone: "info",
      text: t("remuda {server} is on the server; this page runs {page}.", { server: stale, page: __REMUDA_VERSION__ }),
      action: (
        <button type="button" className="btn sm primary" onClick={() => location.reload()}>
          {t("Reload")}
        </button>
      ),
    });
  const release = latest?.remuda;
  const local = machines.find((m) => m.id === "local");
  if (release && version && newer(version, release) && releaseSeen !== release && !offline)
    toasts.push({
      id: "release",
      tone: "info",
      text: t("remuda {release} is out; this one is {version}.", { release, version }),
      action: local && (
        <button type="button" className="btn sm primary" onClick={() => actions.openPanel("release")}>
          {t("Download & install")}
        </button>
      ),
      onDismiss: () => setReleaseSeen(release),
    });
  // the chats a reboot stopped: one toast per machine and episode, the same Resume as the ⓘ's
  for (const m of machines) {
    const stopped = m.stopped ?? [];
    if (!stopped.length) continue;
    const key = episodeKey(m.id, stopped);
    if (recoverSeen.includes(key)) continue;
    toasts.push({
      id: `recover-${m.id}`,
      tone: "warn",
      text: `${m.label}: ${stoppedWords(stopped.length)}`,
      action: (
        <button
          type="button"
          className="btn sm primary"
          onClick={() => {
            setRecoverSeen(remember(recoverSeen, key));
            void resumeStopped(m.id, stopped.length);
          }}
        >
          {t("Resume")}
        </button>
      ),
      onDismiss: () => setRecoverSeen(remember(recoverSeen, key)),
    });
  }
  if (away)
    toasts.push({
      id: "away",
      tone: "info",
      text: away,
      action: (
        <button
          type="button"
          className="btn sm primary"
          onClick={() => {
            actions.setAway(null);
            actions.openPanel("standup");
          }}
        >
          {t("Standup")}
        </button>
      ),
      onDismiss: () => actions.setAway(null),
    });
  if (notice) toasts.push({ id: "notice", tone: notice.error ? "error" : "success", text: notice.text, onDismiss: actions.clearNotice });
  if (tailscale?.state === "needs_login" && tailscale.login_url)
    toasts.push({
      id: "tailscale",
      tone: "info",
      text: t("Tailscale: this remuda is not on your tailnet yet."),
      action: (
        <a className="btn sm primary" href={tailscale.login_url} target="_blank" rel="noreferrer">
          {t("Log in")}
        </a>
      ),
    });
  if (tailscale?.state === "error") toasts.push({ id: "tailscale", tone: "error", text: `Tailscale: ${tailscale.error}` }); // the error is the daemon's own words
  return toasts;
}
