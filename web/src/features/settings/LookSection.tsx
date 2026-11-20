// Settings › Look: the theme's mode, its dark and light themes and the accent, the text size and the code font, the
// density, the panels' widths and the status bar, and whether all of that follows the person to their other devices.

import { setAccentPref, setDarkFlavorPref, setDensityPref, setLightFlavorPref, setMonoPref, setTextSizePref, setThemePref, useAccent, useDarkFlavorPref, useDensity, useFlavor, useLightFlavorPref, useMono, useTextSize, useThemePref } from "@/app/theme";
import { actions } from "@/app/store";
import { useStatusBarPrefs } from "@/features/statusbar/prefs";
import { t } from "@/lib/i18n";
import { DENSITY_NAMES, MONO_FONTS, monoName, monoStack, TEXT_SIZE_NAMES, TEXT_SIZES, type Density, type MonoFont } from "@/lib/look";
import { usePref } from "@/lib/prefs";
import { ACCENTS, accentName, DARK_FLAVORS, LIGHT_FLAVORS, swatches, themeChoiceName } from "@/lib/theme";
import { Icon, resetPanels, Select, Tip } from "@/ui";
import { ThemePicker } from "./ThemePicker";
import { SYNC_PREF } from "./useSyncPrefs";

const DENSITIES: Density[] = ["comfortable", "compact"];
const MONO_CHOICES: MonoFont[] = ["", ...MONO_FONTS];

export default function LookSection() {
  const themePref = useThemePref();
  const darkFlavor = useDarkFlavorPref();
  const lightFlavor = useLightFlavorPref();
  const accent = useAccent();
  const palette = swatches(useFlavor());
  const textSize = useTextSize();
  const density = useDensity();
  const mono = useMono();
  const [statusBar, setStatusBar] = useStatusBarPrefs();
  const [sync, setSync] = usePref<boolean>(SYNC_PREF, false);
  return (
    <section className="settings-section look-settings">
      <h3>{t("Look")}</h3>
      <h4 className="settings-sub">{t("Colours")}</h4>
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
      <h4 className="settings-sub">{t("Text")}</h4>
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
          {t("Code & terminal font")}
          <small>{t("The ones installed on this device show in their own face; the rest fall back to the system's monospace")}</small>
        </span>
        <Select value={mono} onChange={setMonoPref} options={MONO_CHOICES.map((f) => ({ value: f, label: monoName(f), style: { fontFamily: monoStack(f) } }))} />
      </label>
      <h4 className="settings-sub">{t("Layout")}</h4>
      <label className="settings-row">
        <span>
          {t("Density")}
          <small>{t("Compact tightens the rows, the tabs and the gaps on a wide screen with a mouse")}</small>
        </span>
        <Select value={density} onChange={setDensityPref} options={DENSITIES.map((d) => ({ value: d, label: DENSITY_NAMES[d] }))} />
      </label>
      <div className="settings-row">
        <span>
          {t("Panels")}
          <small>{t("The sidebar, the files rail, the review queue and a task's detail keep the width you drag them to")}</small>
        </span>
        <button
          type="button"
          className="btn sm"
          onClick={() => {
            resetPanels();
            actions.notify(t("The panels are back to their usual widths"), { brief: true });
          }}
        >
          {t("Reset widths")}
        </button>
      </div>
      <label className="settings-row">
        <span>
          {t("Status bar")}
          <small>{t("The plan limits, the pane's branch, its machine's load and ports along the foot of the page, on a desk with a mouse")}</small>
        </span>
        <input type="checkbox" checked={statusBar.on} onChange={(e) => setStatusBar({ on: e.target.checked })} />
      </label>
      <label className="settings-row">
        <span>
          {t("Plan limits: what remains")}
          <small>{t("The status bar says what is left of each window instead of what is used; the colours still follow what is used")}</small>
        </span>
        <input type="checkbox" checked={statusBar.remaining} onChange={(e) => setStatusBar({ remaining: e.target.checked })} />
      </label>
      <h4 className="settings-sub">{t("Between devices")}</h4>
      <label className="settings-row">
        <span>
          {t("Sync these settings between devices")}
          <small>{t("The theme, the density, the code font, the sidebar, the status bar, the review and the panels travel through this remuda; alerts, the text sizes and the prompt history stay on the device")}</small>
        </span>
        <input type="checkbox" checked={sync} onChange={(e) => setSync(e.target.checked)} />
      </label>
    </section>
  );
}
