// The Wall mode: every agent on one screen, a tile each — its state, its last turns or its terminal as it is, a
// line to answer — in a grid that fits the window, or one of them given the whole wall with the rest in a strip under it. app composes: the tile is ./Tile over features/chat/Tail, which panes make a tile
// and in what order is features/roster/wall.

import { useEffect, useMemo, type CSSProperties } from "react";
import { DEFAULT_RADAR, type RadarPrefs } from "@/features/roster/radar";
import { moving, useClock, useSpinner } from "@/features/roster/useClocks";
import { useRoster } from "@/features/roster/useRoster";
import { DEFAULT_WALL, TILE_MIN, tiles, withFocus, type Tile, type WallPrefs, type WallView } from "@/features/roster/wall";
import { usePref } from "@/lib/prefs";
import { Icon } from "@/ui";
import { popOut, popOutSupported } from "./pip";
import { actions } from "./store";
import { TileView } from "./Tile";

const VIEWS: { view: WallView; label: string; title: string }[] = [
  { view: "chat", label: "Chat", title: "Each tile the last turns of its conversation" },
  { view: "screen", label: "Screen", title: "Each tile its terminal as it is, scaled to fit" },
];

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
  const [focus, setFocus] = usePref<string>("wall.focus", "");
  const { big, rest } = useMemo(() => withFocus(list.shown, focus), [list.shown, focus]);
  useEffect(() => {
    if (!big) return;
    const onKey = (event: KeyboardEvent) => {
      if (event.key !== "Escape" || (event.target as HTMLElement | null)?.closest("input, textarea, select, [contenteditable]") || document.querySelector(".modal-scrim, .sel-pop")) return;
      setFocus("");
    };
    document.addEventListener("keydown", onKey);
    return () => document.removeEventListener("keydown", onKey);
  }, [big, setFocus]);
  const n = list.shown.length;
  const tile = (t: Tile, brief = false) => (
    <TileView
      key={t.key}
      tile={t}
      view={prefs.view}
      radar={radar}
      step={step}
      now={now}
      brief={brief}
      focused={t === big}
      onFocus={() => setFocus(t === big ? "" : t.key)}
      onHide={() => setHidden((h) => [...h.filter((k) => k !== t.key), t.key])}
      onPopOut={popOutSupported ? () => void popOut({ machineId: t.machine.id, paneId: t.pane.id }) : undefined}
    />
  );
  return (
    <main className="wall">
      <div className="wall-bar">
        <span className="wall-count">
          {n} agent{n === 1 ? "" : "s"}
        </span>
        <button
          type="button"
          className={`btn sm${prefs.needsMe ? " on" : ""}`}
          aria-pressed={prefs.needsMe}
          title="Only the agents waiting for you: a question open, a watchdog note, a finish you have not looked at"
          onClick={() => setPrefs({ ...prefs, needsMe: !prefs.needsMe })}
        >
          Needs me{prefs.needsMe && list.calm ? ` (${list.calm})` : ""}
        </button>
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
        <span className="wall-views">
          {VIEWS.map((v) => (
            <button key={v.view} type="button" className={`btn sm${prefs.view === v.view ? " on" : ""}`} aria-pressed={prefs.view === v.view} title={v.title} onClick={() => setPrefs({ ...prefs, view: v.view })}>
              {v.label}
            </button>
          ))}
        </span>
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
          <p>{machines.some((m) => m.state === "connected") ? (list.calm ? "Nobody needs you right now." : list.hidden || list.idle ? "Every agent is hidden or idle." : "No agent runs anywhere.") : "No machine connected."}</p>
          <button type="button" className="btn" onClick={() => actions.openDialog({ kind: "workspace", machineId: "local" })}>
            Start an agent
          </button>
        </div>
      ) : big ? (
        <>
          <div className="wall-focus">{tile(big)}</div>
          {rest.length > 0 && <div className="wall-strip">{rest.map((t) => tile(t, true))}</div>}
        </>
      ) : (
        <div className="wall-grid" style={{ "--tile-min": `${TILE_MIN[prefs.size]}px` } as CSSProperties}>
          {list.shown.map((t) => tile(t))}
        </div>
      )}
    </main>
  );
}
