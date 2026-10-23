// TileView is one agent as the wall and the pop-out show it: its head as the sidebar row reads it — mark, logo,
// label, workspace · machine, the pull request it named, context % · cost — its state line (how long, and what it runs this second), then the
// tail of its chat, or its terminal as it is, with a line to answer.

import Tail, { TailAnswer, TileState } from "@/features/chat/Tail";
import { displayOf, markOf, rowLabel, stateWords, type RadarPrefs } from "@/features/roster/radar";
import { hueStyle } from "@/features/roster/hue";
import type { Tile, WallView } from "@/features/roster/wall";
import { Screen } from "@/features/terminal/Screen";
import { fillClass, usd } from "@/features/usage/format";
import { openUrl } from "@/lib/host";
import { Icon, Logo } from "@/ui";
import { brandColor } from "@/ui/brand";
import { folderOf } from "./current";
import { actions } from "./store";

export function TileView({ tile, view = "chat", radar, step, now, brief = false, focused = false, onFocus, onHide, onPopOut }: {
  tile: Tile;
  view?: WallView;
  radar: RadarPrefs;
  step: number;
  now: number;
  brief?: boolean; // the strip under a focused tile: the head and the state line only
  focused?: boolean; // this tile has the whole wall
  onFocus?: () => void; // the wall's focus button: give this tile the wall, or the wall back
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
  const screen = view === "screen";
  const open = () => {
    actions.selectPane(machine.id, pane.id);
    actions.setLens(screen ? "terminal" : "chat");
    window.focus();
  };
  const dir = folderOf(tile);
  const words = alert ?? stateWords(d, pane.radar?.last_active, now, pane.radar?.since);
  const settled = !alert && (d === "done" || d.startsWith("idle"));
  const said = settled ? pane.said : undefined;
  const pr = pane.pr;
  return (
    <section className={`tile is-${d}${alert ? " is-alert" : ""}${brief ? " brief" : ""}${machine.id !== "local" ? " has-hue" : ""}`} style={hueStyle(machine.id)} aria-label={`${pane.agent} in ${where}`}>
      <header className="tile-head">
        <button type="button" className="linkish tile-open" title={`${words}\n${pane.cwd}\nOpen the ${screen ? "terminal" : "chat"} in Agents`} onClick={open}>
          <span className="radar-mark" style={markStyle}>
            {alert ? "!" : markOf(d, step)}
          </span>
          <Logo agent={pane.agent} />
          <span className="tile-label">{rowLabel(pane, tab, session.name, radar)}</span>
          <span className="tile-where">{where}</span>
        </button>
        {pr && (
          <button type="button" className="linkish tile-pr" title={`${pr.url}\nOpen in a new tab`} onClick={() => openUrl(pr.url)}>
            <Icon name="external-link" size={12} /> {pr.label}
          </button>
        )}
        {usage && usage.window > 0 && (
          <span className={`radar-ctx tile-usage${fillClass(fill)}`} title={`Context ${Math.round(100 * fill)}% full · ${usage.name} · ${usd(usage.cost)} API-equivalent so far`}>
            {Math.round(100 * fill)}% · {usd(usage.cost)}
          </span>
        )}
        {onFocus && (
          <button type="button" className={`icon-btn sm tile-focus${focused ? " on" : ""}`} title={focused ? "Back to the grid (Esc)" : "Focus: this agent takes the wall, the others wait in a strip"} aria-label={focused ? "Back to the grid" : "Focus this agent"} aria-pressed={focused} onClick={onFocus}>
            <Icon name={focused ? "minimize" : "maximize"} size={14} />
          </button>
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
      {brief ? (
        <TileState words={words} said={said} />
      ) : screen ? (
        <>
          <TileState words={words} said={said} />
          <Screen machineId={machine.id} paneId={pane.id} cols={pane.cols} busy={pane.status === "run" || pane.status === "input"} active onOpen={open} />
          <TailAnswer machineId={machine.id} paneId={pane.id} status={pane.status} dir={dir} active />
        </>
      ) : (
        <Tail machineId={machine.id} paneId={pane.id} status={pane.status} dir={dir} words={words} settled={settled} said={said} active />
      )}
    </section>
  );
}
