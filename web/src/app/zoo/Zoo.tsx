// The Wall's zoo: every agent as a little creature (./ZooCreature) — its species by vendor, its hat by model, a trinket
// of its own, its pose, girth, coins, flag and babies saying what it does — standing in a pen per machine whose weather
// is the machine's body, under the sky of the viewer's hour, with the day told along the foot. Between readings the
// resting ones clap for a finish, a merge sets off fireworks, a compaction puffs, the eyes follow the pointer, and with
// the voices on each calls out. A grid of pens that fills the screen, or office mode's one large beside a column of
// small ones; the pop-out shows one large.

import { useMemo, useRef, type CSSProperties } from "react";
import { displayOf, type RadarPrefs } from "@/features/roster/radar";
import { useRoster } from "@/features/roster/useRoster";
import { useSharing } from "@/features/roster/useSharing";
import type { Tile, WallPrefs } from "@/features/roster/wall";
import { folderOf as sessionFolder } from "@/features/roster/workspaces";
import { CREATURE_SIZE, bubbleOf, cellMin, lookOf } from "@/features/roster/zoo";
import { joinsApplause, tickerFacts } from "@/features/roster/zooLife";
import { pensOf, skyOf, weatherOf, weatherWords } from "@/features/roster/zooPens";
import { outfits, wearersOf } from "@/features/roster/zooWear";
import { hueStyle } from "@/features/roster/hue";
import { t } from "@/lib/i18n";
import { usePref } from "@/lib/prefs";
import { basename } from "@/lib/text";
import type { Office } from "../useOffice";
import { Boxes, Fireworks, Sky, WeatherMarks } from "./Sky";
import { ZooCreature, type Life } from "./ZooCreature";
import { ZooFoot, useDay } from "./ZooFoot";
import { useGaze } from "./useGaze";
import { useQuestions } from "./useQuestions";
import { useZooLife } from "./useZooLife";
import { useVoices, wakeVoices } from "./voices";

export function Zoo({ shown, size, radar, now, office }: { shown: Tile[]; size: WallPrefs["size"]; radar: RadarPrefs; now: number; office: Office | null }) {
  const share = useSharing();
  const root = useRef<HTMLDivElement>(null);
  useGaze(root);
  const [voices, setVoices] = usePref<boolean>("zoo.voices", false);
  useVoices(voices);
  const life = useZooLife(shown, radar, now, voices);
  const { machines } = useRoster();
  const dressed = useMemo(() => outfits(wearersOf(machines)), [machines]);
  const hiddenTile = (tile: Tile) => share.sessionHidden(tile.machine.id, tile.session);
  const askers = shown.filter((tile) => !hiddenTile(tile) && displayOf(tile.pane, radar, now) === "blocked").map((tile) => ({ key: tile.key, machine: tile.machine.id, pane: tile.pane.id }));
  const questions = useQuestions(askers);
  const day = useDay();
  const night = skyOf(new Date(now).getHours()) === "night";
  const folderName = (tile: Tile) => {
    const real = basename(sessionFolder(tile.session)) || tile.session.name;
    return hiddenTile(tile) ? share.folderName(tile.machine.id, sessionFolder(tile.session), real) : real;
  };
  const merged = shown.filter((tile) => !hiddenTile(tile) && tile.pane.pr?.status?.state === "MERGED").length;
  const facts = tickerFacts(day, merged, (machine, pane) => {
    const tile = shown.find((candidate) => candidate.machine.id === machine && candidate.pane.id === pane);
    return tile ? folderName(tile) : null;
  });
  const lifeOf = (tile: Tile): Life => {
    const display = displayOf(tile.pane, radar, now);
    const pose = lookOf(display, tile.pane.radar?.alert, !!tile.pane.limit?.hit_at).pose;
    return {
      outfit: dressed.get(tile.key),
      cheer: life.cheering !== null && life.cheering !== tile.key && joinsApplause(pose),
      puff: life.puffs.has(tile.key),
      question: questions.get(tile.key),
      night,
    };
  };
  const talks = (tile: Tile) => !hiddenTile(tile) && (questions.has(tile.key) || bubbleOf(tile.pane, displayOf(tile.pane, radar, now), tile.pane.radar?.alert) !== "");
  const toggleVoices = () => {
    if (!voices) wakeVoices();
    setVoices(!voices);
  };
  const featured = office ? (shown.find((tile) => tile.key === office.key) ?? null) : null;
  return (
    <div className={`zoo${night ? " is-night" : ""}`} ref={root}>
      <Sky sky={skyOf(new Date(now).getHours())} />
      {office && featured ? (
        <div className="wall-office zoo-office">
          <div className="wall-office-main zoo-featured">
            <ZooCreature tile={featured} radar={radar} now={now} mode="big" life={lifeOf(featured)} />
          </div>
          <div className="wall-office-cards zoo-cards" aria-label={t("The other agents; a click pins one")}>
            {shown
              .filter((tile) => tile !== featured)
              .map((tile) => (
                <ZooCreature key={tile.key} tile={tile} radar={radar} now={now} mode="small" pinned={tile.key === office.pinned} onPick={() => office.pin(tile.key)} life={lifeOf(tile)} />
              ))}
          </div>
        </div>
      ) : (
        <div className="zoo-pens" style={{ "--zoo-cell": `${cellMin(size)}px`, "--zoo-size": `${CREATURE_SIZE[size]}px` } as CSSProperties}>
          {pensOf(shown).map((pen) => {
            const machine = pen.tiles[0]!.machine;
            const weather = weatherOf(machine.health, machine.awake);
            const words = weatherWords(machine.health, weather);
            const label = share.machineLabel(machine);
            // the bubbles' row is kept in a pen only while some creature in it talks, so a quiet pen has no empty band
            const talking = pen.tiles.some(talks);
            const marks = `${weather.heat ? ` is-${weather.heat}` : ""}${weather.storm ? " is-storm" : ""}${weather.dust ? " is-dust" : ""}${weather.lamp ? " is-lamp" : ""}`;
            return (
              <section key={pen.machineId} className={`zoo-pen${marks}`} style={{ "--pen-n": pen.tiles.length, ...hueStyle(machine.id) } as CSSProperties} aria-label={label}>
                <header className="zoo-pen-sign">
                  <span className="zoo-pen-name">{label}</span>
                  <WeatherMarks weather={weather} />
                  {words && <span className="zoo-pen-words">{words}</span>}
                </header>
                <div className="zoo-grid">
                  {pen.tiles.map((tile) => (
                    <ZooCreature key={tile.key} tile={tile} radar={radar} now={now} bubbleRow={talking} life={lifeOf(tile)} />
                  ))}
                </div>
                {weather.boxes && <Boxes />}
              </section>
            );
          })}
        </div>
      )}
      {life.fireworks && <Fireworks />}
      <ZooFoot facts={facts} voices={voices} onVoices={toggleVoices} />
    </div>
  );
}
