// Settings › General: the page's language, the way to this device's alerts and to the keyboard sheet, and which remuda
// this is.

import { readLangPref, setLangPref } from "@/app/lang";
import { actions } from "@/app/store";
import { LANG_NAMES, LANGS, t } from "@/lib/i18n";
import { Icon, Select } from "@/ui";

export default function GeneralSection({ onClose }: { onClose: () => void }) {
  const after = (run: () => void) => () => {
    onClose();
    run();
  };
  return (
    <section className="settings-section general-settings">
      <h3>{t("General")}</h3>
      <label className="settings-row">
        <span>
          {t("Language")}
          <small>{t("The page's own words; the agents' answers, the pushes and the reports stay as they are")}</small>
        </span>
        <Select
          value={readLangPref()}
          onChange={setLangPref}
          options={[{ value: "system", label: t("Follow the browser"), icon: <Icon name="monitor" size={14} /> }, ...LANGS.map((l) => ({ value: l, label: LANG_NAMES[l] }))]}
        />
      </label>
      <div className="settings-row">
        <span>
          {t("Alerts on this device")}
          <small>{t("Pushes, quiet hours, read aloud")}</small>
        </span>
        <button type="button" className="btn sm" onClick={after(() => actions.openPanel("alerts"))}>
          {t("Open")}
        </button>
      </div>
      <div className="settings-row">
        <span>
          {t("Keyboard shortcuts")}
          <small>{t("Every key the page answers (also behind ?)")}</small>
        </span>
        <button type="button" className="btn sm" onClick={after(() => actions.openDialog({ kind: "keys" }))}>
          {t("Show")}
        </button>
      </div>
      <div className="settings-row">
        <span>
          remuda {__REMUDA_VERSION__}
          <small>{t("The updates of every machine sit under its ⓘ in the sidebar")}</small>
        </span>
      </div>
    </section>
  );
}
