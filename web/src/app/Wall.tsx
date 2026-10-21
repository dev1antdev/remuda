// The Wall mode: every agent on one screen, a tile each — its state, its last turns, a line to answer — in a
// grid that fits the window. app composes: the tile's chat is features/chat/Tail, which panes make a tile and
// in what order is features/roster/wall.

import { useMemo, type CSSProperties } from "react";
import Tail from "@/features/chat/Tail";
import { DEFAULT_RADAR, displayOf, markOf, rowLabel, type RadarPrefs } from "@/features/roster/radar";
import { stateWords } from "@/features/roster/rows";
import { moving, useClock, useSpinner } from "@/features/roster/useClocks";
import { useRoster } from "@/features/roster/useRoster";
import { DEFAULT_WALL, TILE_MIN, tiles, type Tile, type WallPrefs } from "@/features/roster/wall";
import { fillClass, usd } from "@/features/usage/format";
import { usePref } from "@/lib/prefs";
import { Icon, Logo } from "@/ui";
import { brandColor } from "@/ui/brand";
import { folderOf } from "./current";
import { actions } from "./store";

const SIZES: { size: WallPrefs["size"]; title: string }[] = [
  { size: "s", title: "Small tiles: the most on screen" },
  { size: "m", title: "Medium tiles" },
  { size: "l", title: "Large tiles: the most of each conversation" },
];

export default function Wall() {
  const { machines } = useRoster();
  const [prefs, setPrefs] = usePref<WallPrefs>("wall", DEFAULT_WALL);
  const [hidden, setHidden] = usePref<string[]>("wall.hidden", []);
  const [radar] = usePref<RadarPrefs>("radar", DEFAULT_RADAR);
  const now = useClock(machines);
  const step = useSpinner(moving(machines));
  const list = useMemo(() => tiles(machines, hidden, prefs, radar, now), [machines, hidden, prefs, radar, now]);
  const n = list.shown.length;
  return (
    <main className="wall">
      <div className="wall-bar">
        <span className="wall-count">
          {n} agent{n === 1 ? "" : "s"}
        </span>
        <button
          type="button"
          className={`btn sm${prefs.hideIdle ? " on" : ""}`}
          aria-pressed={prefs.hideIdle}
          title={`Hide the agents idle for over ${radar.freshMinutes} min (the sidebar's fresh tier)`}
          onClick={() => setPrefs({ ...prefs, hideIdle: !prefs.hideIdle })}
        >
          Hide idle{prefs.hideIdle && list.idle ? ` (${list.idle})` : ""}
        </button>
        {list.hidden > 0 && (
          <button type="button" className="btn sm" title="Bring back the tiles hidden by hand" onClick={() => setHidden([])}>
            Show {list.hidden} hidden
          </button>
        )}
        <span className="wall-spacer" />
        <span className="wall-sizes">
          {SIZES.map((s) => (
            <button key={s.size} type="button" className={`btn sm${prefs.size === s.size ? " on" : ""}`} aria-pressed={prefs.size === s.size} title={s.title} onClick={() => setPrefs({ ...prefs, size: s.size })}>
              {s.size.toUpperCase()}
            </button>
          ))}
        </span>
        <button type="button" className="btn sm" title="Start an agent in a new workspace" onClick={() => actions.openDialog({ kind: "workspace", machineId: "local" })}>
          <Icon name="plus" size={12} /> Agent
        </button>
      </div>
      {n === 0 ? (
        <div className="placeholder">
          <p>{machines.some((m) => m.state === "connected") ? (list.hidden || list.idle ? "Every agent is hidden or idle." : "No agent runs anywhere.") : "No machine connected."}</p>
          <button type="button" className="btn" onClick={() => actions.openDialog({ kind: "workspace", machineId: "local" })}>
            Start an agent
          </button>
        </div>
      ) : (
        <div className="wall-grid" style={{ "--tile-min": `${TILE_MIN[prefs.size]}px` } as CSSProperties}>
          {list.shown.map((t) => (
            <TileView key={t.key} tile={t} radar={radar} step={step} now={now} onHide={() => setHidden((h) => [...h.filter((k) => k !== t.key), t.key])} />
          ))}
        </div>
      )}
    </main>
  );
}

// TileView is one agent: its head as the sidebar row reads it, then its tail of a chat
function TileView({ tile, radar, step, now, onHide }: { tile: Tile; radar: RadarPrefs; step: number; now: number; onHide: () => void }) {
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
        <button type="button" className="icon-btn sm tile-hide" title="Hide this tile (Show hidden brings it back)" aria-label="Hide this tile" onClick={onHide}>
          <Icon name="eye-off" size={14} />
        </button>
      </header>
      <Tail machineId={machine.id} paneId={pane.id} status={pane.status} dir={folderOf(tile)} active />
    </section>
  );
}
