import { useEffect, useRef, useState } from "react";
import { errMsg, readAlertPrefs, writePref } from "./prefs";
import {
  disablePush,
  enablePush,
  getPushState,
  sendTestPush,
  updatePrefs,
  type Prefs,
  type PushState,
} from "./push";
import Select from "./Select";
import { Dialog, Notice, useTimedNotice } from "./ui";

// Push alerts for this device: enable, what to be told about, a test push.
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

  return (
    <Dialog title="Alerts on this device" icon="bell" onClose={onClose} className="alerts-dialog">
      {!state && <Notice tone="hint">Checking…</Notice>}
      {state && !state.supported && <Notice tone="warn">{state.reason}</Notice>}
      {state?.supported && state.permission === "denied" && (
        <Notice tone="warn">Notifications are blocked for this site — allow them in the browser's site settings.</Notice>
      )}

      {state?.supported && (
        <div className="alerts-row">
          <span>
            {state.subscribed ? "Push alerts are on" : "Push alerts are off"}
            <small>Delivered with the app closed, through your browser's push service.</small>
          </span>
          <button
            type="button"
            className={state.subscribed ? "btn" : "btn primary"}
            disabled={busy || state.permission === "denied"}
            onClick={() =>
              void run(
                state.subscribed ? disablePush : () => enablePush(prefs),
                state.subscribed ? "Alerts turned off." : "Alerts are on — try a test push.",
              )
            }
          >
            {state.subscribed ? "Turn off" : "Turn on"}
          </button>
        </div>
      )}

      <label className="alerts-opt">
        <input type="checkbox" checked={prefs.input} onChange={(e) => changePrefs({ ...prefs, input: e.target.checked })} />
        <span>
          When an agent <b>needs you</b>
          <small>A question or approval still open after 10 s — answered at the desk, it's never sent.</small>
        </span>
      </label>
      <label className="alerts-opt">
        <input type="checkbox" checked={prefs.watchdog ?? true} onChange={(e) => changePrefs({ ...prefs, watchdog: e.target.checked })} />
        <span>
          When an agent <b>looks stuck</b>
          <small>Working 20 min with nothing written, waiting for an answer 15 min, or looping on one tool call — with an Esc button on the notification.</small>
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
            { value: "long", label: "long turns" },
            { value: "always", label: "always" },
          ]}
        />
      </label>

      {state?.subscribed && state.endpoint && (
        <button
          type="button"
          className="btn sm alerts-test"
          disabled={busy}
          onClick={() => void run(() => sendTestPush(state.endpoint!), "Test push sent — it should appear in a moment.")}
        >
          Send a test push
        </button>
      )}
      {err && <Notice tone="error" onDismiss={() => setErr("")}>{err}</Notice>}
      {done && <Notice tone="success">{done}</Notice>}
    </Dialog>
  );
}
