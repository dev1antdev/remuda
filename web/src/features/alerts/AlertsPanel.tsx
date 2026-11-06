import { useEffect, useRef, useState } from "react";
import { errMsg } from "@/lib/text";
import { deviceZone, readAlertPrefs, readQuietWindow, tellAppPrefs } from "@/features/alerts/prefs";
import { usePref, writePref } from "@/lib/prefs";
import { disablePush, enablePush, getPushState, sendTestPush, updatePrefs, type Prefs, type PushState, type Quiet } from "@/features/alerts/push";
import { inShell } from "@/lib/shell";
import { speak, useVoices, voiceSupported } from "@/features/alerts/useVoice";
import { DEFAULT_VOICE, type VoicePrefs } from "@/features/alerts/voice";
import WebhooksSection from "@/features/alerts/WebhooksSection";
import DeviceStatus from "@/features/alerts/DeviceStatus";
import { testAlert } from "@/features/alerts/onpage";
import { actions } from "@/app/store";
import { alertSections } from "@/features/alerts/sections";
import { useRoster } from "@/features/roster/useRoster";
import Select from "@/ui/Select";
import { DEFAULT_OPEN_LENS, OPEN_LENS_KEY, type OpenLens } from "@/app/openLens";
import { Dialog, Notice, useTimedNotice } from "@/ui";
import { t } from "@/lib/i18n";

// Push alerts for this device: enable, what to be told about, quiet hours, a test push; and the browser's voice reading the agents aloud.
export default function AlertsPanel({ onClose }: { onClose: () => void }) {
  const [state, setState] = useState<PushState | null>(null);
  const [prefs, setPrefs] = useState<Prefs>(readAlertPrefs);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState("");
  const [done, showDone] = useTimedNotice(6000);
  const [looks, setLooks] = useState(0); // every change of the subscription or a test reads the device's status again

  const reload = async () => {
    setState(await getPushState());
    setLooks((n) => n + 1);
  };
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
    if (tellAppPrefs(next) || !state?.subscribed) return;
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
  const [openLens, setOpenLens] = usePref<OpenLens>(OPEN_LENS_KEY, DEFAULT_OPEN_LENS);
  const voices = useVoices();
  const native = inShell(); // the Mac app is the device: its alerts come over its own socket, not Web Push
  const { viewOnly, machines } = useRoster();
  const shown = alertSections({ native, viewOnly, rosterKnown: machines.length > 0 }); // the local machine is always there once the roster answered
  const voiceOptions = [{ value: "", label: t("The browser's default voice") }, ...voices.map((v) => ({ value: v.name, label: v.name, hint: v.lang, group: v.lang }))];
  const sayOne = () => speak(/[\u0400-\u04ff]/.test(voice.voice ?? "") ? "api: Все тесты прошли, ветка запушена." : t("api: The tests pass and the branch is pushed."), "", voice.voice);

  return (
    <Dialog title={t("Alerts on this device")} icon="bell" onClose={onClose} className="alerts-dialog">
      {viewOnly && <Notice tone="hint">{t("This screen is for viewing: no push reaches it, and the webhooks are set from a signed-in device. It can read the agents aloud.")}</Notice>}
      {native && !viewOnly && <Notice tone="hint">{t("Notifications come through the Mac app")}</Notice>}
      {shown.push && !state && <Notice tone="hint">{t("Checking…")}</Notice>}
      {shown.push && state && !state.supported && <Notice tone="warn">{state.reason}</Notice>}
      {shown.push && state?.supported && <DeviceStatus state={state} version={looks} />}

      {shown.push && state?.supported && (
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
      {err && (
        <Notice tone="error" onDismiss={() => setErr("")}>
          {err}
        </Notice>
      )}

      <label className="alerts-opt alerts-open">
        <span>
          <b>{t("A notification opens")}</b>
          <small>{t("The pane a tapped notification, the Mac app's menu, an Activity row or Alt+N brings up shows this lens; the sidebar, the tabs and the Wall keep their own ways.")}</small>
        </span>
        <Select
          value={openLens}
          onChange={setOpenLens}
          options={[
            { value: "terminal", label: t("the terminal") },
            { value: "chat", label: t("the chat") },
            { value: "keep", label: t("the lens it was on") },
          ]}
        />
      </label>
      {shown.kinds && (
        <>
          <label className="alerts-opt">
            <input type="checkbox" checked={prefs.input} onChange={(e) => changePrefs({ ...prefs, input: e.target.checked })} />
            <span>
              {t("When an agent")} <b>{t("needs you")}</b>
              <small>{t("A question or approval still open after 10 s — answered at the desk, it's never sent.")}</small>
            </span>
          </label>
          <label className="alerts-opt">
            <input type="checkbox" checked={prefs.watchdog ?? true} onChange={(e) => changePrefs({ ...prefs, watchdog: e.target.checked })} />
            <span>
              {t("When an agent")} <b>{t("looks stuck")}</b>
              <small>{t("Working 20 min with nothing written, waiting for an answer 15 min, or looping on one tool call — with an Esc button on the notification.")}</small>
            </span>
          </label>
          <label className="alerts-opt">
            <span>
              {t("When an agent")} <b>{t("finishes")}</b>
              <small>{t("Every finish 10 s after it; or only turns that worked a minute or more, a minute after.")}</small>
            </span>
            <Select
              value={prefs.done}
              onChange={(done) => changePrefs({ ...prefs, done })}
              options={[
                { value: "always", label: t("every finish") },
                { value: "long", label: t("turns over a minute") },
                { value: "off", label: t("never") },
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
            <span>{t("to")}</span>
            <input type="time" className="sm" value={quietWindow.to} disabled={!prefs.quiet} aria-label={t("Quiet until")} onChange={(e) => changeWindow({ ...quietWindow, to: e.target.value })} />
          </div>
        </>
      )}

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

      {!viewOnly && !native && (
        <button
          type="button"
          className="btn sm alerts-test"
          disabled={busy}
          onClick={() => {
            actions.addAlert(testAlert());
            const endpoint = state?.subscribed ? state.endpoint : undefined;
            if (!endpoint) {
              showDone(t("A test is up in the corner of this page. Turn on push for a system notification too."));
              return;
            }
            void run(() => sendTestPush(endpoint), t("A test is up in the corner of this page, and a system notification should appear in a moment."));
          }}
        >
          {t("Send a test")}
        </button>
      )}
      {done && <Notice tone="success">{done}</Notice>}
      {shown.webhooks && <WebhooksSection />}
    </Dialog>
  );
}
