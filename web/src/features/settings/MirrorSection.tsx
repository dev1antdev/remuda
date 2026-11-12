// The Inside herdr section of Settings: remuda's figures as herdr tokens (on by default) with the snippet herdr's
// config needs to show them, remuda's alerts as herdr's own toasts too (off by default) with their quiet hours, and a
// test toast on every machine. One cockpit's switches for every machine it reaches.

import { useEffect, useState } from "react";
import { actions } from "@/app/store";
import { DEFAULT_QUIET, deviceZone } from "@/features/alerts/prefs";
import { copyText } from "@/lib/host";
import { t } from "@/lib/i18n";
import { useQuery } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { Notice } from "@/ui";
import { elsewhereWords, outcomesTone, outcomeWords, quietZone, withQuiet } from "./mirror";
import { fetchMirror, saveMirror, testMirror, type MirrorSettings, type ToastOutcome } from "./mirrorApi";

export default function MirrorSection() {
  const { data: status, mutate } = useQuery(fetchMirror, []);
  const [failure, setFailure] = useState("");
  const [testing, setTesting] = useState(false);
  const [tested, setTested] = useState<ToastOutcome[] | null>(null);
  const [quietWindow, setQuietWindow] = useState(DEFAULT_QUIET);
  useEffect(() => {
    if (status?.quiet) setQuietWindow({ from: status.quiet.from, to: status.quiet.to });
  }, [status?.quiet?.from, status?.quiet?.to]); // eslint-disable-line react-hooks/exhaustive-deps

  if (!status) return null;
  const settings: MirrorSettings = { figures: status.figures, toasts: status.toasts, ...(status.quiet ? { quiet: status.quiet } : {}) };
  const zone = quietZone(settings, deviceZone());
  const save = async (next: MirrorSettings) => {
    setFailure("");
    try {
      const saved = await saveMirror(next);
      mutate(() => saved);
    } catch (e) {
      setFailure(errMsg(e));
    }
  };
  const changeWindow = (key: "from" | "to", value: string) => {
    if (!/^\d\d:\d\d$/.test(value)) return;
    const next = { ...quietWindow, [key]: value };
    setQuietWindow(next);
    if (settings.quiet) void save(withQuiet(settings, true, next, zone));
  };
  const test = async () => {
    setTesting(true);
    setFailure("");
    try {
      setTested(await testMirror());
    } catch (e) {
      setFailure(errMsg(e));
    } finally {
      setTesting(false);
    }
  };
  const copy = (text: string) =>
    void copyText(text).then((ok) => actions.notify(ok ? t("Copied for herdr's config.toml") : t("It could not be copied; select the text instead"), { brief: true, error: !ok }));
  const outcomes = tested ?? status.last ?? [];

  return (
    <section className="settings-section mirror-settings">
      <h3>{t("Inside herdr")}</h3>
      {status.closed && <Notice tone="info">{t("This remuda is a scratch instance beside the real one: it writes nothing into herdr, so the two never fight over the same panes.")}</Notice>}
      {(status.elsewhere ?? []).map((where) => (
        <Notice key={where.machine} tone="info">
          {elsewhereWords(where)}
        </Notice>
      ))}
      <label className="settings-row">
        <span>
          {t("Show remuda's figures in herdr")}
          <small>
            {t(
              "On by default. Each agent's cost, context, model, pull request, task, the watchdog's note and its last answer go to herdr as tokens on its pane, a workspace's review and cost on the workspace — written when they move, gone within five minutes once remuda stops. Off clears them.",
            )}
          </small>
        </span>
        <input type="checkbox" checked={settings.figures} onChange={(e) => void save({ ...settings, figures: e.target.checked })} />
      </label>
      <p className="backup-hint">
        {t(
          "herdr shows a token only where a sidebar row of its config.toml names it, and a row whose tokens have no value disappears. Put these rows there — in place of the rows of a [ui.sidebar.agents] or [ui.sidebar.spaces] it already has, since a table written twice makes herdr ignore the whole file — check it with herdr config check, then run herdr server reload-config:",
        )}
      </p>
      <pre className="config-snippet">{status.snippet}</pre>
      <div className="backup-actions">
        <button type="button" className="btn sm" onClick={() => copy(status.snippet)}>
          {t("Copy")}
        </button>
        <span className="backup-hint">{t("The watchdog's note also stands in state_text, in place of the state it is about.")}</span>
      </div>
      <label className="settings-row">
        <span>
          {t("Alerts inside herdr too")}
          <small>
            {t(
              "Off by default. A question, a finish after a turn over a minute and the watchdog's notes also pop up as herdr's own toast on the machine the agent runs on — never over the tab herdr has in front, never for a muted workspace. One switch for every machine: herdr shows a toast only where its config.toml turns its toasts on, which it does not by default.",
            )}
          </small>
        </span>
        <input type="checkbox" checked={settings.toasts} onChange={(e) => void save({ ...settings, toasts: e.target.checked })} />
      </label>
      <p className="backup-hint">
        {t("Set delivery under the [ui.toast] its config.toml may already have — a second one makes herdr ignore the whole file — or add the table where there is none:")}
      </p>
      <pre className="config-snippet">{status.toast_snippet}</pre>
      <label className="alerts-opt">
        <input type="checkbox" checked={!!settings.quiet} onChange={(e) => void save(withQuiet(settings, e.target.checked, quietWindow, zone))} />
        <span>
          <b>{t("Quiet hours")}</b>
          <small>{t("No toast pops up between the two times, by the clock of {zone}.", { zone })}</small>
        </span>
      </label>
      <div className="alerts-quiet">
        <input type="time" className="sm" value={quietWindow.from} disabled={!settings.quiet} aria-label={t("Quiet from")} onChange={(e) => changeWindow("from", e.target.value)} />
        <span>{t("to")}</span>
        <input type="time" className="sm" value={quietWindow.to} disabled={!settings.quiet} aria-label={t("Quiet until")} onChange={(e) => changeWindow("to", e.target.value)} />
      </div>
      <div className="backup-actions">
        <button type="button" className="btn" disabled={testing || !!status.closed} onClick={() => void test()}>
          {testing ? t("Sending…") : t("Send a test toast")}
        </button>
        <span className="backup-hint">{t("to every connected machine, whatever the switches say")}</span>
      </div>
      {failure && <Notice tone="error">{failure}</Notice>}
      {outcomes.length > 0 && (
        <Notice tone={outcomesTone(outcomes)}>
          <ul className="mirror-outcomes">
            {outcomes.map((o) => (
              <li key={o.machine}>{outcomeWords(o)}</li>
            ))}
          </ul>
        </Notice>
      )}
    </section>
  );
}
