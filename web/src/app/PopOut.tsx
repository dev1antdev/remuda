// PopOut draws the popped-out pane into the small window: the same tile as on the wall — or its creature, large, when
// the wall shows the zoo — through a portal, so it shares the roster, the store and every handler of this page.

import { createPortal } from "react-dom";
import { usePref } from "@/lib/prefs";
import { DEFAULT_RADAR, type RadarPrefs } from "@/features/roster/radar";
import { moving, useClock, useSpinner } from "@/features/roster/useClocks";
import { useRoster } from "@/features/roster/useRoster";
import { PortalHost } from "@/ui";
import { DEFAULT_WALL, tileKey, type WallPrefs } from "@/features/roster/wall";
import { useAllPanes } from "./current";
import { popOutWindow } from "./pip";
import { app, type Selection } from "./store";
import { TileView } from "./Tile";
import { ZooCreature } from "./zoo/ZooCreature";
import { t } from "@/lib/i18n";

export default function PopOut() {
  const popout = app.use((s) => s.popout);
  const win = popOutWindow();
  if (!popout || !win) return null;
  // the tile's menus and tooltips open in this window too, not in the page behind it
  return createPortal(
    <PortalHost.Provider value={win.document.body}>
      <Popped popout={popout} />
    </PortalHost.Provider>,
    win.document.body,
  );
}

// Popped is the window's content; its clocks tick only while a window is open
function Popped({ popout }: { popout: Selection }) {
  const { machines } = useRoster();
  const panes = useAllPanes();
  const [radar] = usePref<RadarPrefs>("radar", DEFAULT_RADAR);
  const [wall] = usePref<WallPrefs>("wall", DEFAULT_WALL);
  const now = useClock(machines);
  const step = useSpinner(moving(machines));
  const located = panes.find((x) => x.machine.id === popout.machineId && x.pane.id === popout.paneId);
  return (
    <div className="popout-body">
      {located && wall.view === "zoo" && located.pane.agent ? (
        <div className="zoo-popout">
          <ZooCreature tile={{ ...located, key: tileKey(located.machine.id, located.pane.id) }} radar={radar} now={now} mode="big" />
        </div>
      ) : located ? (
        <TileView tile={{ ...located, key: tileKey(located.machine.id, located.pane.id) }} view={wall.view} radar={radar} step={step} now={now} />
      ) : (
        <div className="placeholder">
          <p>{t("That pane is gone.")}</p>
        </div>
      )}
    </div>
  );
}
