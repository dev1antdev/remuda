// TileView is one agent as the wall and the pop-out show it: its head as the sidebar row reads it — mark, logo,
// label, workspace · machine, context % · cost — then the tail of its chat with a line to answer.

import Tail from "@/features/chat/Tail";
import { displayOf, markOf, rowLabel, type RadarPrefs } from "@/features/roster/radar";
import { stateWords } from "@/features/roster/rows";
import type { Tile } from "@/features/roster/wall";
import { fillClass, usd } from "@/features/usage/format";
import { Icon, Logo } from "@/ui";
import { brandColor } from "@/ui/brand";
import { folderOf } from "./current";
import { actions } from "./store";

export function TileView({ tile, radar, step, now, onHide, onPopOut }: {
  tile: Tile;
  radar: RadarPrefs;
  step: number;
  now: number;
  onHide?: () => void; // the wall's eye
  onPopOut?: () => void; // the wall's pop-out, when the browser can
}) {
  const { machine, session, tab, pane } = tile;
  const d = displayOf(pane, radar, now);
  const alert = pane.radar?.alert;
  const markStyle = d === "working" && !alert ? { color: brandColor(pane.agent) } : undefined;
  const usage = pane.usage;
  const fill = usage && usage.window > 0 ? usage.context / usage.window : 0;
  const where = `${session.name}${machine.id !== "local" ? ` · ${machine.label}` : ""}`;
  const open = () => {
    actions.selectPane(machine.id, pane.id);
    actions.setLens("chat");
    window.focus();
  };
  return (
    <section className={`tile is-${d}${alert ? " is-alert" : ""}`} aria-label={`${pane.agent} in ${where}`}>
      <header className="tile-head">
        <button type="button" className="linkish tile-open" title={`${alert ?? stateWords(d, pane.radar?.last_active, now)}\n${pane.cwd}\nOpen in Agents`} onClick={open}>
          <span className="radar-mark" style={markStyle}>
            {alert ? "!" : markOf(d, step)}
          </span>
          <Logo agent={pane.agent} />
          <span className="tile-label">{rowLabel(pane, tab, session.name, radar)}</span>
          <span className="tile-where">{where}</span>
        </button>
        {usage && usage.window > 0 && (
          <span className={`radar-ctx tile-usage${fillClass(fill)}`} title={`Context ${Math.round(100 * fill)}% full · ${usage.name} · ${usd(usage.cost)} API-equivalent so far`}>
            {Math.round(100 * fill)}% · {usd(usage.cost)}
          </span>
        )}
        {onPopOut && (
          <button type="button" className="icon-btn sm tile-pop" title="Pop out: a small window above every other app" aria-label="Pop out" onClick={onPopOut}>
            <Icon name="pip" size={14} />
          </button>
        )}
        {onHide && (
          <button type="button" className="icon-btn sm tile-hide" title="Hide this tile (Show hidden brings it back)" aria-label="Hide this tile" onClick={onHide}>
            <Icon name="eye-off" size={14} />
          </button>
        )}
      </header>
      <Tail machineId={machine.id} paneId={pane.id} status={pane.status} dir={folderOf(tile)} active />
    </section>
  );
}
