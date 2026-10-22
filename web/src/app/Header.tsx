// The header: [burger][modes][title — takes the slack][usage pill][lens switch][pop-out][search][bell]. Nothing
// in it moves when state changes: the lens group is anchored right and the pill grows into the title's slack;
// the pop-out button is there whenever the browser can do it, disabled without a pane.

import { basename } from "@/lib/text";
import { machineLabel } from "@/features/roster/options";
import { useRoster } from "@/features/roster/useRoster";
import { fillClass, tok, usd } from "@/features/usage/format";
import { Icon } from "@/ui";
import type { Located } from "./current";
import { popOut, popOutSupported } from "./pip";
import { actions, app, type Lens } from "./store";

export default function Header({ current }: { current: Located | null }) {
  const { machines } = useRoster();
  const mode = app.use((s) => s.mode);
  const lens = app.use((s) => s.lens);
  const drawer = app.use((s) => s.drawer);
  const viewer = app.use((s) => s.viewer);
  const panel = app.use((s) => s.panel);
  return (
    <header className="topbar">
      <button type="button" className="burger icon-btn" onClick={actions.toggleDrawer} title="Menu" aria-expanded={drawer}>
        <Icon name="menu" size={18} />
      </button>
      <nav className="mode-switch">
        <button className={mode === "herdr" ? "on" : ""} onClick={() => actions.setMode("herdr")} title="Your agents and terminals, from herdr">
          Agents
        </button>
        <button className={mode === "wall" ? "on" : ""} onClick={() => actions.setMode("wall")} title="Every agent on one screen, a tile each">
          Wall
        </button>
        <button className={mode === "tasks" ? "on" : ""} onClick={() => actions.setMode("tasks")} title="Your tasks, kept on this remuda">
          Tasks
        </button>
        <button className={mode === "code" ? "on" : ""} onClick={() => actions.setMode("code")} title="VS Code on a machine, through remuda">
          Code
        </button>
      </nav>
      <span className="title">
        {mode !== "herdr" ? null : viewer ? (
          <>
            {viewer.machine !== "local" && <span className="title-machine">{machineLabel(machines, viewer.machine)} ›</span>}
            {basename(viewer.dir)}
            <span className="title-agent"> · transcript</span>
          </>
        ) : current ? (
          <>
            {current.machine.id !== "local" && <span className="title-machine">{current.machine.label} ›</span>}
            {current.session.name}
            <span className="title-agent"> · {current.pane.agent || "shell"}</span>
          </>
        ) : (
          "remuda"
        )}
      </span>
      {mode === "herdr" && current && !viewer && (
        <div className="lenses">
          {current.pane.usage && current.pane.usage.window > 0 && <UsagePill usage={current.pane.usage} />}
          <LensSwitch lens={lens} current={current} />
        </div>
      )}
      {popOutSupported && (
        <button
          type="button"
          className="icon-btn bell"
          title="Pop out this agent: a small window above every other app"
          disabled={!current || !!viewer}
          onClick={() => current && void popOut({ machineId: current.machine.id, paneId: current.pane.id })}
        >
          <Icon name="pip" size={18} />
        </button>
      )}
      <button type="button" className="icon-btn bell" title="Search every conversation (also from ⌘K / Ctrl+K)" onClick={() => actions.openPanel("search")}>
        <Icon name="search" size={18} />
      </button>
      <button type="button" className={`icon-btn bell${panel === "alerts" ? " on" : ""}`} title="Alerts on this device" aria-expanded={panel === "alerts"} onClick={() => actions.togglePanel("alerts")}>
        <Icon name="bell" size={18} />
      </button>
    </header>
  );
}

function UsagePill({ usage }: { usage: NonNullable<Located["pane"]["usage"]> }) {
  const fill = usage.context / usage.window;
  const percent = Math.round(100 * fill);
  return (
    <button
      type="button"
      className={`usage-pill${fillClass(fill)}`}
      title={`${usage.name}: context ${tok(usage.context)} of ${tok(usage.window)} (${percent}%) · ${usage.turns} responses · ${usd(usage.cost)} API-equivalent so far. Click for the usage report.`}
      onClick={() => actions.openPanel("usage")}
    >
      <span className="usage-pill-bar">
        <span style={{ width: `${Math.min(100, 100 * fill)}%` }} />
      </span>
      {percent}%<span className="usage-pill-cost"> · {usd(usage.cost)}</span>
    </button>
  );
}

function LensSwitch({ lens, current }: { lens: Lens; current: Located }) {
  const tab = (id: Lens, label: string, extra?: { disabled?: boolean; title?: string; badge?: number }) => (
    <button role="tab" aria-selected={lens === id} className={lens === id ? "on" : ""} disabled={extra?.disabled} title={extra?.title} onClick={() => actions.setLens(id)}>
      {label}
      {extra?.badge ? <span className="lens-badge">{extra.badge}</span> : null}
    </button>
  );
  return (
    <nav className="lens-switch" role="tablist" aria-label="View">
      {tab("chat", "Chat")}
      {tab("terminal", "Terminal")}
      {tab("changes", "Changes", {
        disabled: !(current.session.dir || current.pane.cwd),
        title: "What changed in this workspace's folder since you last looked",
        badge: current.session.review?.fresh,
      })}
    </nav>
  );
}
