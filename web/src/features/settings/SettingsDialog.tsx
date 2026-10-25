// Settings: the few things the page keeps per device, in one place — the theme, its flavour and accent, the text size,
// the density and the monospace font, how the sidebar reads, the panels' widths, whether those follow the person to
// their other devices, the way to the alerts and the keys, which remuda this is, and the devices signed in to it.

import { setAccentPref, setDarkFlavorPref, setDensityPref, setMonoPref, setTextSizePref, setThemePref, useAccent, useDarkFlavorPref, useDensity, useFlavor, useMono, useTextSize, useThemePref } from "@/app/theme";
import { actions } from "@/app/store";
import DevicesSection from "./DevicesSection";
import { RadarFields } from "@/features/roster/RadarSettings";
import { DEFAULT_RADAR, type RadarPrefs } from "@/features/roster/radar";
import { DENSITY_NAMES, MONO_FONTS, monoName, monoStack, TEXT_SIZE_NAMES, TEXT_SIZES, type Density, type MonoFont } from "@/lib/look";
import { usePref } from "@/lib/prefs";
import { ACCENTS, accentName, DARK_FLAVORS, FLAVOR_NAMES, FLAVORS, swatches } from "@/lib/theme";
import { Dialog, Icon, resetPanels, Select, Tip } from "@/ui";
import { SYNC_PREF } from "./useSyncPrefs";

const DENSITIES: Density[] = ["comfortable", "compact"];
const MONO_CHOICES: MonoFont[] = ["", ...MONO_FONTS];

export default function SettingsDialog({ onClose }: { onClose: () => void }) {
  const themePref = useThemePref();
  const darkFlavor = useDarkFlavorPref();
  const flavor = useFlavor();
  const accent = useAccent();
  const textSize = useTextSize();
  const density = useDensity();
  const mono = useMono();
  const palette = swatches(flavor);
  const [radar, setRadar] = usePref<RadarPrefs>("radar", DEFAULT_RADAR);
  const [sync, setSync] = usePref<boolean>(SYNC_PREF, false);
  const after = (run: () => void) => () => {
    onClose();
    run();
  };
  return (
    <Dialog title="Settings" icon="sliders" onClose={onClose} className="settings-dialog">
      <section className="settings-section">
        <h3>Look</h3>
        <label className="settings-row">
          <span>
            Theme
            <small>Catppuccin: your dark flavour by night, Latte by day</small>
          </span>
          <Select
            value={themePref}
            onChange={setThemePref}
            options={[
              { value: "system", label: "Follow the system", icon: <Icon name="monitor" size={14} /> },
              { value: "dark", label: `${FLAVOR_NAMES[darkFlavor]} (dark)`, icon: <Icon name="moon" size={14} /> },
              { value: "light", label: "Latte (light)", icon: <Icon name="sun" size={14} /> },
            ]}
          />
        </label>
        <label className="settings-row">
          <span>
            Dark flavour
            <small>Frappé is the softest, Mocha the deepest</small>
          </span>
          <Select
            value={darkFlavor}
            onChange={setDarkFlavorPref}
            options={DARK_FLAVORS.map((f) => ({ value: f, label: FLAVOR_NAMES[f], icon: <span className="swatch" style={{ background: FLAVORS[f].base, borderColor: FLAVORS[f].surface2 }} /> }))}
          />
        </label>
        <div className="settings-row">
          <span>
            Accent
            <small>The colour of the buttons, the rings and the marks: {accentName(accent)}</small>
          </span>
          <div className="accent-swatches" role="radiogroup" aria-label="Accent">
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
            Text size
            <small>The page's type scale; the terminal keeps its own size in its View menu</small>
          </span>
          <Select
            value={textSize}
            onChange={setTextSizePref}
            options={TEXT_SIZES.map((s) => ({ value: s, label: TEXT_SIZE_NAMES[s], icon: <Icon name={s === "small" ? "text-smaller" : "text-larger"} size={14} /> }))}
          />
        </label>
        <label className="settings-row">
          <span>
            Density
            <small>Compact tightens the rows, the tabs and the gaps on a wide screen with a mouse</small>
          </span>
          <Select value={density} onChange={setDensityPref} options={DENSITIES.map((d) => ({ value: d, label: DENSITY_NAMES[d] }))} />
        </label>
        <label className="settings-row">
          <span>
            Code &amp; terminal font
            <small>The ones installed on this device show in their own face; the rest fall back to the system's monospace</small>
          </span>
          <Select
            value={mono}
            onChange={setMonoPref}
            options={MONO_CHOICES.map((f) => ({ value: f, label: monoName(f), style: { fontFamily: monoStack(f) } }))}
          />
        </label>
        <div className="settings-row">
          <span>
            Panels
            <small>The sidebar, the files rail, the review queue and a task's detail keep the width you drag them to</small>
          </span>
          <button type="button" className="btn sm" onClick={() => { resetPanels(); actions.notify("The panels are back to their usual widths", { brief: true }); }}>
            Reset widths
          </button>
        </div>
        <label className="settings-row">
          <span>
            Sync these settings between devices
            <small>The theme, the density, the code font, the sidebar, the wall, the review and the panels travel through this remuda; alerts, the text sizes and the prompt history stay on the device</small>
          </span>
          <input type="checkbox" checked={sync} onChange={(e) => setSync(e.target.checked)} />
        </label>
      </section>
      <section className="settings-section">
        <h3>Sidebar</h3>
        <RadarFields prefs={radar} onPrefs={setRadar} />
      </section>
      <section className="settings-section">
        <h3>More</h3>
        <div className="settings-row">
          <span>
            Alerts on this device
            <small>Pushes, quiet hours, read aloud</small>
          </span>
          <button type="button" className="btn sm" onClick={after(() => actions.openPanel("alerts"))}>
            Open
          </button>
        </div>
        <div className="settings-row">
          <span>
            Keyboard shortcuts
            <small>Every key the page answers (also behind ?)</small>
          </span>
          <button type="button" className="btn sm" onClick={after(() => actions.openDialog({ kind: "keys" }))}>
            Show
          </button>
        </div>
        <div className="settings-row">
          <span>
            remuda {__REMUDA_VERSION__}
            <small>The updates of every machine sit under its ⓘ in the sidebar</small>
          </span>
        </div>
      </section>
      <DevicesSection />
    </Dialog>
  );
}
