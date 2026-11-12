// The player: one recording's frames drawn as the Wall's screen view draws a pane — the same ANSI renderer, the font
// fitted to the pane's columns — the clock run at 10×, 30× or 60× the turn's own time or a frame at a time, a scrub bar
// over the turn, a step either way, and Export as GIF. A find's step (jump) takes it to the frame the text appeared in,
// paused, the row drawn apart; the text found is marked on whatever frame shows it.

import { useEffect, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { useFlavor } from "@/app/theme";
import { clockOf, whenShort } from "@/lib/time";
import { usePref } from "@/lib/prefs";
import { ansiPalette } from "@/lib/theme";
import { t, tn } from "@/lib/i18n";
import { Icon, Tip } from "@/ui";
import { parseAnsi } from "../ansi";
import { ScreenLines, useFitFont, widthOf } from "../Screen";
import type { Recording } from "./api";
import type { Step } from "./found";
import { GifExport } from "./GifExport";
import { markLines } from "./mark";
import { advance, DEFAULT_SPEED, frameAt, playerKey, screensOf, screenText, SPEEDS, stepTo, type Position, type Speed } from "./playback";
import { durationOf, endedWords, speedWords } from "./words";

const SPEED_PREF = "timelapse.speed";
// the screen view's own measures: its rows' line height, the box's padding and border, and the share of the window it may take
const LINE_HEIGHT = 1.25;
const SCREEN_PAD = 14;
const SCREEN_SHARE = 0.56;

// fitHeight is the largest font, in half pixels, at which the rows fit the room the window leaves the box
const fitHeight = (rows: number) => Math.max(2, Math.floor(((window.innerHeight * SCREEN_SHARE - SCREEN_PAD) / (rows * LINE_HEIGHT)) * 2) / 2);

export function Player({ recording, jump = null, marked = "" }: { recording: Recording; jump?: Step | null; marked?: string }) {
  const screens = useMemo(() => screensOf(recording.frames), [recording]);
  const times = useMemo(() => recording.frames.map((f) => f.at), [recording]);
  const duration = Math.max(durationOf(recording), times[times.length - 1] ?? 0);
  const [stored, setSpeed] = usePref<Speed>(SPEED_PREF, DEFAULT_SPEED);
  const speed = SPEEDS.includes(stored) ? stored : DEFAULT_SPEED;
  const [pos, setPos] = useState<Position>({ t: 0, carry: 0 });
  const [playing, setPlaying] = useState(true);
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    setPos({ t: jump ? jump.at : 0, carry: 0 });
    setPlaying(!jump);
  }, [recording, jump]);
  useEffect(() => {
    if (!jump) box.current?.focus({ preventScroll: true }); // Space and the arrows are the player's from the start; a find keeps its field
  }, [recording]); // eslint-disable-line react-hooks/exhaustive-deps
  usePlayback(playing, speed, times, duration, pos, setPos, () => setPlaying(false));

  const index = frameAt(times, pos.t);
  const flavor = useFlavor();
  const palette = useMemo(() => ansiPalette(flavor), [flavor]);
  const drawn = useMemo(() => parseAnsi(screenText(screens[index]), palette), [screens, index, palette]);
  const lines = useMemo(() => markLines(drawn, marked), [drawn, marked]);
  const rows = useMemo(() => Math.max(recording.rows, 1, ...recording.frames.map((f) => f.lines)), [recording]);
  // the whole screen in sight: as wide as the pane's columns allow, and no taller than the room the box has
  const fontSize = Math.min(useFitFont(box, widthOf(recording.cols, lines)), fitHeight(rows));
  const at = new Date(Date.parse(recording.start) + (times[index] ?? 0));

  const toggle = () => {
    if (!playing && pos.t >= duration) setPos({ t: 0, carry: 0 });
    setPlaying(!playing);
  };
  const step = (by: 1 | -1) => {
    setPlaying(false);
    setPos({ t: stepTo(times, pos.t, by), carry: 0 });
  };
  const onKey = (e: KeyboardEvent) => {
    if (e.defaultPrevented) return;
    const target = e.target instanceof HTMLElement ? e.target : null;
    const place = target?.closest("input, select, textarea, [role=listbox], [contenteditable=true]") ? "field" : target?.closest("button") ? "button" : "screen";
    const action = playerKey(e.key, e.altKey || e.ctrlKey || e.metaKey, place);
    if (action === null) return;
    e.preventDefault();
    if (action === "toggle") toggle();
    else step(action);
  };
  return (
    <div className="timelapse" onKeyDown={onKey}>
      <div className="screen timelapse-screen" ref={box} tabIndex={0} role="region" style={{ fontSize, height: `calc(${rows * LINE_HEIGHT}em + ${SCREEN_PAD}px)` }} aria-label={t("Frame {n} of {total}", { n: index + 1, total: times.length })}>
        <ScreenLines lines={lines} current={jump && jump.frame === index ? jump.line : -1} />
      </div>
      <div className="timelapse-bar">
        <Tip text={playing ? t("Pause (Space)") : t("Play (Space)")}>
          <button type="button" className="icon-btn" aria-label={playing ? t("Pause") : t("Play")} onClick={toggle}>
            <Icon name={playing ? "pause" : "play"} size={16} />
          </button>
        </Tip>
        <Tip text={t("The frame before (←)")}>
          <button type="button" className="icon-btn" aria-label={t("The frame before")} disabled={index === 0} onClick={() => step(-1)}>
            <Icon name="skip-back" size={16} />
          </button>
        </Tip>
        <Tip text={t("The frame after (→)")}>
          <button type="button" className="icon-btn" aria-label={t("The frame after")} disabled={index >= times.length - 1} onClick={() => step(1)}>
            <Icon name="skip-forward" size={16} />
          </button>
        </Tip>
        <input
          type="range"
          className="timelapse-scrub"
          min={0}
          max={Math.max(1, duration)}
          step={1000}
          value={Math.min(pos.t, duration)}
          aria-label={t("Where in the turn")}
          onChange={(e) => setPos({ t: Number(e.target.value), carry: 0 })}
        />
        <span className="timelapse-clock" title={t("Recorded at {when}", { when: whenShort(at) })}>
          {clockOf(pos.t)} / {clockOf(duration)}
        </span>
      </div>
      <div className="timelapse-bar">
        <div className="timelapse-speeds" role="group" aria-label={t("Speed")}>
          {SPEEDS.map((s) => (
            <button key={s} type="button" className={`btn sm${s === speed ? " on" : ""}`} aria-pressed={s === speed} onClick={() => setSpeed(s)}>
              {speedWords(s)}
            </button>
          ))}
        </div>
        <span className="timelapse-meta">
          {tn(times.length, "{n} frame", "{n} frames")} · {endedWords(recording.ended)}
        </span>
        <GifExport recording={recording} screens={screens} times={times} speed={speed} palette={palette} />
      </div>
    </div>
  );
}

// usePlayback runs the clock a screen refresh at a time while playing, from wherever the scrub bar or a step put it,
// and stops it at the turn's end
function usePlayback(playing: boolean, speed: Speed, times: number[], duration: number, pos: Position, setPos: (pos: Position) => void, onEnd: () => void) {
  const latest = useRef({ pos, onEnd });
  latest.current = { pos, onEnd };
  useEffect(() => {
    if (!playing) return;
    let frame = 0;
    let last = performance.now();
    const tick = (now: number) => {
      const next = advance(latest.current.pos, Math.min(250, now - last), speed, times, duration);
      last = now;
      latest.current.pos = next.pos;
      setPos(next.pos);
      if (next.ended) latest.current.onEnd();
      else frame = requestAnimationFrame(tick);
    };
    frame = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(frame);
  }, [playing, speed, times, duration, setPos]);
}
