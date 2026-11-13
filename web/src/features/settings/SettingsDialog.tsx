// Settings: the few things the page keeps per device, in one place — the theme's mode, its dark and light themes and the
// accent, the text size, the density and the monospace font, how the sidebar reads, the panels' widths, whether those
// follow the person to their other devices, the way to the alerts and the keys, which remuda this is, what it shows
// inside herdr, and the devices signed in to it; and, kept on the cockpit, the sleep and resume switches, the Jira site, the backups and the agents'
// timelapses.

import { setAccentPref, setDarkFlavorPref, setDensityPref, setLightFlavorPref, setMonoPref, setTextSizePref, setThemePref, useAccent, useDarkFlavorPref, useDensity, useFlavor, useLightFlavorPref, useMono, useTextSize, useThemePref } from "@/app/theme";
import { actions } from "@/app/store";
import BackupSection from "./BackupSection";
import TimelapseSection from "./TimelapseSection";
import DevicesSection from "./DevicesSection";
import JiraSection from "@/features/jira/JiraSection";
import MirrorSection from "./MirrorSection";
import SleepSection from "@/features/sleep/SleepSection";
import { useRoster } from "@/features/roster/useRoster";
import { RadarFields } from "@/features/roster/RadarSettings";
import { DEFAULT_RADAR, type RadarPrefs } from "@/features/roster/radar";
import { DENSITY_NAMES, MONO_FONTS, monoName, monoStack, TEXT_SIZE_NAMES, TEXT_SIZES, type Density, type MonoFont } from "@/lib/look";
import { usePref } from "@/lib/prefs";
import { ACCENTS, accentName, DARK_FLAVORS, LIGHT_FLAVORS, swatches, themeChoiceName } from "@/lib/theme";
import { ThemePicker } from "./ThemePicker";
import { Dialog, Icon, resetPanels, Select, Tip } from "@/ui";
import { SYNC_PREF } from "./useSyncPrefs";
import { LANG_NAMES, LANGS, t } from "@/lib/i18n";
import { readLangPref, setLangPref } from "@/app/lang";

const DENSITIES: Density[] = ["comfortable", "compact"];
const MONO_CHOICES: MonoFont[] = ["", ...MONO_FONTS];

export default function SettingsDialog({ onClose }: { onClose: () => void }) {
  const themePref = useThemePref();
  const darkFlavor = useDarkFlavorPref();
  const lightFlavor = useLightFlavorPref();
  const flavor = useFlavor();
  const accent = useAccent();
  const textSize = useTextSize();
  const density = useDensity();
  const mono = useMono();
  const palette = swatches(flavor);
  const [radar, setRadar] = usePref<RadarPrefs>("radar", DEFAULT_RADAR);
  const [sync, setSync] = usePref<boolean>(SYNC_PREF, false);
  const { viewOnly } = useRoster();
  const after = (run: () => void) => () => {
    onClose();
    run();
  };
  return (
    <Dialog title={t("Settings")} icon="sliders" onClose={onClose} className="settings-dialog">
      <section className="settings-section">
        <h3>{t("Look")}</h3>
        <label className="settings-row">
          <span>
            {t("Theme")}
            <small>{t("Following the system, the page wears your dark theme when it is dark and your light theme when it is light")}</small>
          </span>
          <Select
            value={themePref}
            onChange={setThemePref}
            options={[
              { value: "system", label: themeChoiceName("system", darkFlavor, lightFlavor), icon: <Icon name="monitor" size={14} /> },
              { value: "dark", label: themeChoiceName("dark", darkFlavor, lightFlavor), icon: <Icon name="moon" size={14} /> },
              { value: "light", label: themeChoiceName("light", darkFlavor, lightFlavor), icon: <Icon name="sun" size={14} /> },
            ]}
          />
        </label>
        <div className="settings-row settings-themes">
          <span>
            {t("Dark theme")}
            <small>{t("The page, the terminal, the diagrams and the editor wear it whenever the page is dark")}</small>
          </span>
          <ThemePicker label={t("Dark theme")} flavors={DARK_FLAVORS} value={darkFlavor} accent={accent} onPick={setDarkFlavorPref} />
        </div>
        <div className="settings-row settings-themes">
          <span>
            {t("Light theme")}
            <small>{t("…and this one whenever it is light")}</small>
          </span>
          <ThemePicker label={t("Light theme")} flavors={LIGHT_FLAVORS} value={lightFlavor} accent={accent} onPick={setLightFlavorPref} />
        </div>
        <div className="settings-row">
          <span>
            {t("Accent")}
            <small>{t("The colour of the buttons, the rings and the marks: {accent}", { accent: accentName(accent) })}</small>
          </span>
          <div className="accent-swatches" role="radiogroup" aria-label={t("Accent")}>
            {ACCENTS.map((a) => (
              <Tip key={a} text={accentName(a)}>
                <button
                  type="button"
                  role="radio"
                  aria-checked={a === accent}
                  className={`icon-btn sm accent-swatch${a === accent ? " on" : ""}`}
                  aria-label={accentName(a)}
                  onClick={() => setAccentPref(a)}
                >
                  <span className="swatch" style={{ background: palette[a] }} />
                </button>
              </Tip>
            ))}
          </div>
        </div>
        <label className="settings-row">
          <span>
            {t("Language")}
            <small>{t("The page's own words; the agents' answers, the pushes and the reports stay as they are")}</small>
          </span>
          <Select
            value={readLangPref()}
            onChange={setLangPref}
            options={[
              { value: "system", label: t("Follow the browser"), icon: <Icon name="monitor" size={14} /> },
              ...LANGS.map((l) => ({ value: l, label: LANG_NAMES[l] })),
            ]}
          />
        </label>
        <label className="settings-row">
          <span>
            {t("Text size")}
            <small>{t("The page's type scale; the terminal keeps its own size in its View menu")}</small>
          </span>
          <Select
            value={textSize}
            onChange={setTextSizePref}
            options={TEXT_SIZES.map((s) => ({ value: s, label: TEXT_SIZE_NAMES[s], icon: <Icon name={s === "small" ? "text-smaller" : "text-larger"} size={14} /> }))}
          />
        </label>
        <label className="settings-row">
          <span>
            {t("Density")}
            <small>{t("Compact tightens the rows, the tabs and the gaps on a wide screen with a mouse")}</small>
          </span>
          <Select value={density} onChange={setDensityPref} options={DENSITIES.map((d) => ({ value: d, label: DENSITY_NAMES[d] }))} />
        </label>
        <label className="settings-row">
          <span>
            {t("Code & terminal font")}
            <small>{t("The ones installed on this device show in their own face; the rest fall back to the system's monospace")}</small>
          </span>
          <Select
            value={mono}
            onChange={setMonoPref}
            options={MONO_CHOICES.map((f) => ({ value: f, label: monoName(f), style: { fontFamily: monoStack(f) } }))}
          />
        </label>
        <div className="settings-row">
          <span>
            {t("Panels")}
            <small>{t("The sidebar, the files rail, the review queue and a task's detail keep the width you drag them to")}</small>
          </span>
          <button type="button" className="btn sm" onClick={() => { resetPanels(); actions.notify(t("The panels are back to their usual widths"), { brief: true }); }}>
            {t("Reset widths")}
          </button>
        </div>
        <label className="settings-row">
          <span>
            {t("Sync these settings between devices")}
            <small>{t("The theme, the density, the code font, the sidebar, the wall, the review and the panels travel through this remuda; alerts, the text sizes and the prompt history stay on the device")}</small>
          </span>
          <input type="checkbox" checked={sync} onChange={(e) => setSync(e.target.checked)} />
        </label>
      </section>
      <section className="settings-section">
        <h3>{t("Sidebar")}</h3>
        <RadarFields prefs={radar} onPrefs={setRadar} />
      </section>
      <section className="settings-section">
        <h3>{t("More")}</h3>
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
      {!viewOnly && <SleepSection />}
      {!viewOnly && <MirrorSection />}
      {!viewOnly && <JiraSection />}
      {!viewOnly && <BackupSection />}
      {!viewOnly && <TimelapseSection />}
      <DevicesSection />
    </Dialog>
  );
}
