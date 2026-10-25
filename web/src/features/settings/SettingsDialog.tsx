// Settings: the few things the page keeps per device, in one place — the theme, how the sidebar reads, the panels'
// widths, the way to the alerts and the keys, and which remuda this is.

import { setThemePref, useThemePref } from "@/app/theme";
import { actions } from "@/app/store";
import { RadarFields } from "@/features/roster/RadarSettings";
import { DEFAULT_RADAR, type RadarPrefs } from "@/features/roster/radar";
import { usePref } from "@/lib/prefs";
import { Dialog, Icon, resetPanels, Select } from "@/ui";

export default function SettingsDialog({ onClose }: { onClose: () => void }) {
  const themePref = useThemePref();
  const [radar, setRadar] = usePref<RadarPrefs>("radar", DEFAULT_RADAR);
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
            <small>Catppuccin Frappé by night, Latte by day</small>
          </span>
          <Select
            value={themePref}
            onChange={setThemePref}
            options={[
              { value: "system", label: "Follow the system", icon: <Icon name="monitor" size={14} /> },
              { value: "dark", label: "Frappé (dark)", icon: <Icon name="moon" size={14} /> },
              { value: "light", label: "Latte (light)", icon: <Icon name="sun" size={14} /> },
            ]}
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
    </Dialog>
  );
}
