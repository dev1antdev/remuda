import { useEffect, useRef, useState } from "react";
import { errMsg } from "@/lib/text";
import { deviceZone, readAlertPrefs, readQuietWindow } from "@/features/alerts/prefs";
import { usePref, writePref } from "@/lib/prefs";
import { disablePush, enablePush, getPushState, sendTestPush, updatePrefs, type Prefs, type PushState, type Quiet } from "@/features/alerts/push";
import { inShell } from "@/lib/shell";
import { speak, useVoices, voiceSupported } from "@/features/alerts/useVoice";
import { DEFAULT_VOICE, type VoicePrefs } from "@/features/alerts/voice";
import WebhooksSection from "@/features/alerts/WebhooksSection";
import Select from "@/ui/Select";
import { Dialog, Notice, useTimedNotice } from "@/ui";
import { t } from "@/lib/i18n";

// Push alerts for this device: enable, what to be told about, quiet hours, a test push; and the browser's voice reading the agents aloud.
export default function AlertsPanel({ onClose }: { onClose: () => void }) {
  const [state, setState] = useState<PushState | null>(null);
  const [prefs, setPrefs] = useState<Prefs>(readAlertPrefs);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [done, showDone] = useTimedNotice(6000);

  const reload = async () => setState(await getPushState());
  useEffect(() => {
    void reload();
  }, []);

  const run = async (fn: () => Promise<void>, ok?: string) => {
    setBusy(true);
    setErr("");
    try {
      await fn();
      if (ok) showDone(ok);
    } catch (e) {
      setErr(errMsg(e));
    } finally {
      setBusy(false);
      await reload();
    }
  };

  const queue = useRef(Promise.resolve()); // serialized, so the last toggle sent is the one that holds
  const changePrefs = (next: Prefs) => {
    setPrefs(next);
    writePref("alerts", next);
    if (!state?.subscribed) return;
    queue.current = queue.current.then(() => updatePrefs(next)).catch((e) => setErr(errMsg(e)));
  };

  const zone = deviceZone();
  const [quietWindow, setQuietWindow] = useState(readQuietWindow);
  const changeQuiet = (on: boolean) => changePrefs({ ...prefs, quiet: on ? { ...quietWindow, tz: zone } : undefined });
  const changeWindow = (next: Omit<Quiet, "tz">) => {
    if (!/^\d\d:\d\d$/.test(next.from) || !/^\d\d:\d\d$/.test(next.to)) return; // a field being cleared: keep the last window
    setQuietWindow(next);
    writePref("alerts.quiet", next);
    if (prefs.quiet) changePrefs({ ...prefs, quiet: { ...next, tz: zone } });
  };

  const [voice, setVoice] = usePref<VoicePrefs>("alerts.voice", DEFAULT_VOICE);
  const voices = useVoices();
  const native = inShell(); // the Mac app is the device: its alerts come over its own socket, not Web Push
  const voiceOptions = [{ value: "", label: t("The browser's default voice") }, ...voices.map((v) => ({ value: v.name, label: v.name, hint: v.lang, group: v.lang }))];
  const sayOne = () => speak(/[\u0400-\u04ff]/.test(voice.voice ?? "") ? "api: Все тесты прошли, ветка запушена." : t("api: The tests pass and the branch is pushed."), "", voice.voice);

  return (
    <Dialog title={t("Alerts on this device")} icon="bell" onClose={onClose} className="alerts-dialog">
      {native && <Notice tone="hint">{t("Notifications come through the Mac app")}</Notice>}
      {!native && !state && <Notice tone="hint">{t("Checking…")}</Notice>}
      {!native && state && !state.supported && <Notice tone="warn">{state.reason}</Notice>}
      {!native && state?.supported && state.permission === "denied" && (
        <Notice tone="warn">{t("Notifications are blocked for this site — allow them in the browser's site settings.")}</Notice>
      )}

      {!native && state?.supported && (
        <div className="alerts-row">
          <span>
            {state.subscribed ? t("Push alerts are on") : t("Push alerts are off")}
            <small>{t("Delivered with the app closed, through your browser's push service.")}</small>
          </span>
          <button
            type="button"
            className={state.subscribed ? "btn" : "btn primary"}
            disabled={busy || state.permission === "denied"}
            onClick={() =>
              void run(
                state.subscribed ? disablePush : () => enablePush(prefs),
                state.subscribed ? t("Alerts turned off.") : t("Alerts are on — try a test push."),
              )
            }
          >
            {state.subscribed ? t("Turn off") : t("Turn on")}
          </button>
        </div>
      )}

      <label className="alerts-opt">
        <input type="checkbox" checked={prefs.input} onChange={(e) => changePrefs({ ...prefs, input: e.target.checked })} />
        <span>
          When an agent <b>{t("needs you")}</b>
          <small>{t("A question or approval still open after 10 s — answered at the desk, it's never sent.")}</small>
        </span>
      </label>
      <label className="alerts-opt">
        <input type="checkbox" checked={prefs.watchdog ?? true} onChange={(e) => changePrefs({ ...prefs, watchdog: e.target.checked })} />
        <span>
          When an agent <b>{t("looks stuck")}</b>
          <small>{t("Working 20 min with nothing written, waiting for an answer 15 min, or looping on one tool call — with an Esc button on the notification.")}</small>
        </span>
      </label>
      <label className="alerts-opt">
        <span>
          When an agent <b>finishes</b>
          <small>Turns ≥ 1 min, a minute after; "always" means every turn, after 10 s.</small>
        </span>
        <Select
          value={prefs.done}
          onChange={(done) => changePrefs({ ...prefs, done })}
          options={[
            { value: "off", label: "never" },
            { value: "long", label: t("long turns") },
            { value: "always", label: "always" },
          ]}
        />
      </label>
      <label className="alerts-opt">
        <input type="checkbox" checked={!!prefs.quiet} onChange={(e) => changeQuiet(e.target.checked)} />
        <span>
          <b>{t("Quiet hours")}</b>
          <small>{t("Nothing is sent to this device between the two times, by its own clock ({zone}). Activity keeps what happened meanwhile.", { zone })}</small>
        </span>
      </label>
      <div className="alerts-quiet">
        <input type="time" className="sm" value={quietWindow.from} disabled={!prefs.quiet} aria-label={t("Quiet from")} onChange={(e) => changeWindow({ ...quietWindow, from: e.target.value })} />
        <span>to</span>
        <input type="time" className="sm" value={quietWindow.to} disabled={!prefs.quiet} aria-label={t("Quiet until")} onChange={(e) => changeWindow({ ...quietWindow, to: e.target.value })} />
      </div>

      <label className="alerts-opt">
        <input type="checkbox" checked={voice.on} disabled={!voiceSupported} onChange={(e) => setVoice({ ...voice, on: e.target.checked })} />
        <span>
          <b>{t("Read aloud")}</b> {t("on this device")}
          <small>
            {voiceSupported
              ? t("The browser's own voice says what an agent answered when it finishes, that one waits for you, a watchdog note — while this page is open and in front. Nothing passes through remuda.")
              : t("This browser has no speech synthesis.")}
          </small>
        </span>
      </label>
      <div className="alerts-quiet alerts-voice">
        <Select value={voice.voice ?? ""} options={voiceOptions} onChange={(name) => setVoice({ ...voice, voice: name || undefined })} />
        <button type="button" className="btn sm" disabled={!voice.on} onClick={sayOne}>
          {t("Say a line")}
        </button>
      </div>

      {state?.subscribed && state.endpoint && (
        <button
          type="button"
          className="btn sm alerts-test"
          disabled={busy}
          onClick={() => void run(() => sendTestPush(state.endpoint!), "Test push sent — it should appear in a moment.")}
        >
          {t("Send a test push")}
        </button>
      )}
      {err && <Notice tone="error" onDismiss={() => setErr("")}>{err}</Notice>}
      {done && <Notice tone="success">{done}</Notice>}
      <WebhooksSection />
    </Dialog>
  );
}
