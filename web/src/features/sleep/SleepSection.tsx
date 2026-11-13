// Settings › Sleep and resume: wake a sleeping workspace when it is opened, and resume the chats a reboot stopped by
// themselves. Both kept on the cockpit, so they hold on every device; both off until switched on.

import { useState } from "react";
import { t } from "@/lib/i18n";
import { useQuery } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { Notice } from "@/ui";
import { fetchSleepSettings, saveSleepSettings, type SleepSettings } from "./api";

export default function SleepSection() {
  const { data, mutate } = useQuery(fetchSleepSettings, []);
  const [failure, setFailure] = useState("");
  const flip = async (change: Partial<SleepSettings>) => {
    if (!data) return;
    setFailure("");
    try {
      const saved = await saveSleepSettings({ ...data, ...change });
      mutate(() => saved);
    } catch (e) {
      setFailure(errMsg(e));
    }
  };
  return (
    <section className="settings-section sleep-settings">
      <h3>{t("Sleep and resume")}</h3>
      <label className="settings-row">
        <span>
          {t("Wake a sleeping workspace when I open it")}
          <small>{t("A workspace put to sleep from its menu wakes the moment you open one of its panes, its agents back on their conversations. Off, Wake in its menu does it.")}</small>
        </span>
        <input type="checkbox" checked={!!data?.wake_on_open} disabled={!data} onChange={(e) => void flip({ wake_on_open: e.target.checked })} />
      </label>
      <label className="settings-row">
        <span>
          {t("Resume automatically next time")}
          <small>{t("When a machine comes back from a reboot, the chats it stopped start again on their sessions by themselves, a row in the activity saying what came back. Off, a toast and the machine's ⓘ offer them in Resume after restart.")}</small>
        </span>
        <input type="checkbox" checked={!!data?.resume_automatically} disabled={!data} onChange={(e) => void flip({ resume_automatically: e.target.checked })} />
      </label>
      {failure && <Notice tone="error">{failure}</Notice>}
    </section>
  );
}
