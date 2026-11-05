// The Backups section of Settings: the machine the cockpit's state is copied to every night, the time, how many copies
// stay there, the last run and the next, Back up now, and the copies on the machine. Nothing is copied before a machine
// is picked.

import { useEffect, useState } from "react";
import { machineOptions } from "@/features/roster/options";
import { useRoster } from "@/features/roster/useRoster";
import { useQuery } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { whenShort } from "@/lib/time";
import { Notice, Select } from "@/ui";
import { t, tn } from "@/lib/i18n";
import { fetchBackups, listBackups, runBackup, saveBackups, type BackupSettings } from "./backupApi";
import { bytesWords, KEEP_CHOICES, lastWords, nextWords, validTime } from "./backup";

export default function BackupSection() {
  const { machines } = useRoster();
  const { data: status, mutate, reload } = useQuery(fetchBackups, []);
  const [at, setAt] = useState("");
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState("");
  const [look, setLook] = useState(0);
  const machine = status?.settings.machine ?? "";
  const { data: listed, error: listError } = useQuery(() => (machine ? listBackups() : Promise.resolve(null)), [machine, look], { enabled: !!machine });
  useEffect(() => {
    if (status) setAt(status.settings.at);
  }, [status?.settings.at]); // eslint-disable-line react-hooks/exhaustive-deps

  const save = async (next: Partial<BackupSettings>) => {
    if (!status) return;
    setFailure("");
    try {
      const saved = await saveBackups({ ...status.settings, ...next });
      mutate(() => saved);
    } catch (e) {
      setFailure(errMsg(e));
    }
  };
  const run = async () => {
    setBusy(true);
    setFailure("");
    try {
      await runBackup();
    } catch (e) {
      setFailure(errMsg(e));
    } finally {
      setBusy(false);
      reload();
      setLook((n) => n + 1);
    }
  };

  if (!status) return null;
  const files = listed?.files ?? [];
  return (
    <section className="settings-section backup-settings">
      <h3>{t("Backups")}</h3>
      <p className="backup-hint">
        {t(
          "Every night the cockpit's state — the tasks, the schedules, the budgets, the recipes, the webhooks, the settings, the signed-in devices, the agents' keys, the token, the push keys and its Tailscale node — is copied to the machine picked here, into its remuda folder under backups/{host}. `remuda restore` puts a copy back: on a new machine the cockpit keeps its address and every device stays signed in. Never run the old cockpit and the restored one at once.",
          { host: status.host },
        )}
      </p>
      <label className="settings-row">
        <span>
          {t("Machine")}
          <small>{t("pick another machine than the cockpit's own: a dead disk takes its own backups with it")}</small>
        </span>
        <Select value={machine} onChange={(value) => void save({ machine: value })} options={[{ value: "", label: t("No backups") }, ...machineOptions(machines)]} />
      </label>
      <label className="settings-row">
        <span>
          {t("Every night at")}
          <small>{t("the cockpit's own time")}</small>
        </span>
        <input
          type="time"
          className="sm"
          value={at}
          onChange={(e) => {
            setAt(e.target.value);
            if (validTime(e.target.value)) void save({ at: e.target.value });
          }}
        />
      </label>
      <label className="settings-row">
        <span>
          {t("Copies kept")}
          <small>{t("the newest; older ones are removed on the machine")}</small>
        </span>
        <Select value={status.settings.keep} onChange={(keep) => void save({ keep })} options={[...new Set([...KEEP_CHOICES, status.settings.keep])].sort((a, b) => a - b).map((n) => ({ value: n, label: String(n) }))} />
      </label>
      {failure && <Notice tone="error">{failure}</Notice>}
      {status.last && !failure && <Notice tone={status.last.error ? "error" : "success"}>{lastWords(status.last, Date.now(), status.last_good)}</Notice>}
      <div className="backup-actions">
        <button type="button" className="btn" disabled={!machine || busy || status.running} onClick={() => void run()}>
          {busy || status.running ? t("Backing up…") : t("Back up now")}
        </button>
        {machine && status.next && <span className="backup-hint">{nextWords(status.next)}</span>}
      </div>
      {machine && listError && <Notice tone="error">{listError}</Notice>}
      {machine && listed && (
        <div className="backup-files">
          <span className="backup-hint">{files.length ? tn(files.length, "{n} copy in {folder}", "{n} copies in {folder}", { folder: listed.folder }) : t("No copy in {folder} yet", { folder: listed.folder })}</span>
          {files.length > 0 && (
            <ul>
              {files.map((file) => (
                <li key={file.name} title={file.name}>
                  <span>{whenShort(file.at)}</span>
                  <span className="backup-size">{bytesWords(file.size)}</span>
                </li>
              ))}
            </ul>
          )}
        </div>
      )}
    </section>
  );
}
