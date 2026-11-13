// The status bar along the foot of the main area, after Orca's: the agents' plan limits on the left, where the pane on
// screen works in the middle, and on the right the load its machine's herdr panes make, the ports they listen on and
// the machine's name. On a desk with a mouse alone, out of the way in full screen, hidden from the View menu or
// Settings; its segments keep their room, so nothing in it moves when a value does.

import { useRoster } from "@/features/roster/useRoster";
import { hueStyle } from "@/features/roster/hue";
import { useSharing } from "@/features/roster/useSharing";
import { systemWords } from "@/features/roster/system";
import Limits from "@/features/statusbar/LimitsSegment";
import Load from "@/features/statusbar/LoadSegment";
import Place from "@/features/statusbar/PlaceSegment";
import Ports from "@/features/statusbar/PortsSegment";
import { useStatusBarPrefs, useStatusBarScreen } from "@/features/statusbar/prefs";
import { t } from "@/lib/i18n";
import { Tip } from "@/ui";
import { useCurrent } from "./current";
import { actions, app } from "./store";

export default function StatusBar() {
  const [prefs] = useStatusBarPrefs();
  const desk = useStatusBarScreen();
  const full = app.use((s) => s.full);
  const mode = app.use((s) => s.mode);
  const current = useCurrent();
  const { machines, viewOnly } = useRoster();
  const { machineLabel } = useSharing();
  if (!prefs.on || !desk || (full && (mode === "herdr" || mode === "wall"))) return null;
  const machine = current?.machine ?? machines.find((m) => m.state === "connected") ?? machines[0];
  const windows = machine?.os === "windows";
  return (
    <footer className="status-bar" aria-label={t("Status bar")}>
      <Limits machines={machines} remaining={prefs.remaining} onOpen={() => actions.openPanel("usage")} />
      <Place current={current} />
      {machine && (
        <div className="status-machine-side">
          {!viewOnly && <Load key={machine.id} machine={machine} windows={windows} />}
          {!viewOnly && <Ports key={`ports-${machine.id}`} machine={machine} windows={windows} viewOnly={viewOnly} />}
          <Tip text={[machineLabel(machine), stateWords(machine.state), systemWords(machine.os)].filter(Boolean).join(" · ")} side="top">
            <span className="status-item status-host" tabIndex={0} style={hueStyle(machine.id)}>
              <span className={`status-host-dot${machine.id === "local" ? " is-local" : ""}`} aria-hidden="true" />
              <span className="status-host-name">{machineLabel(machine)}</span>
            </span>
          </Tip>
        </div>
      )}
    </footer>
  );
}

const stateWords = (state: string) => (state === "connected" ? t("connected") : state === "connecting" ? t("connecting") : t("away"));
