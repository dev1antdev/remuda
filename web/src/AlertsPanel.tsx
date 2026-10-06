import { useEffect, useState } from "react";
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

// Push alerts for this device: enable, what to be told about, a test push.
export default function AlertsPanel({ onClose }: { onClose: () => void }) {
  const [state, setState] = useState<PushState | null>(null);
  const [prefs, setPrefs] = useState<Prefs>(readAlertPrefs);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<string | null>(null);

  const reload = async () => setState(await getPushState());
  useEffect(() => {
    void reload();
  }, []);

  const run = async (fn: () => Promise<void>, ok?: string) => {
    setBusy(true);
    setMsg(null);
    try {
      await fn();
      if (ok) setMsg(ok);
    } catch (e) {
      setMsg(errMsg(e));
    } finally {
      setBusy(false);
      await reload();
    }
  };

  const changePrefs = (next: Prefs) => {
    setPrefs(next);
    writePref("alerts", next);
    if (state?.subscribed) void run(() => updatePrefs(next));
  };

  return (
    <div className="alerts" onClick={(e) => e.stopPropagation()}>
      <div className="alerts-head">
        <strong>Alerts on this device</strong>
        <button className="icon-btn" onClick={onClose} title="Close">
          ×
        </button>
      </div>

      {!state && <div className="alerts-note">Checking…</div>}
      {state && !state.supported && <div className="alerts-note warn">{state.reason}</div>}
      {state?.supported && state.permission === "denied" && (
        <div className="alerts-note warn">
          Notifications are blocked for this site — allow them in the browser's site settings.
        </div>
      )}

      {state?.supported && (
        <div className="alerts-row">
          <span>
            {state.subscribed ? "Push alerts are on" : "Push alerts are off"}
            <small>Delivered with the app closed, through your browser's push service.</small>
          </span>
          <button
            className={state.subscribed ? "" : "primary"}
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
        <input
          type="checkbox"
          checked={prefs.input}
          onChange={(e) => changePrefs({ ...prefs, input: e.target.checked })}
        />
        <span>
          When an agent <b>needs you</b>
          <small>A question or approval still open after 10 s — answered at the desk, it's never sent.</small>
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
          className="alerts-test"
          disabled={busy}
          onClick={() => void run(() => sendTestPush(state.endpoint!), "Test push sent — it should appear in a moment.")}
        >
          Send a test push
        </button>
      )}
      {msg && <div className="alerts-note">{msg}</div>}
    </div>
  );
}
