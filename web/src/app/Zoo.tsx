// The Wall's zoo: every agent as a little creature in its vendor's hue — its pose and the sign over its head say what it
// does (features/roster/zoo.ts), its name, its folder and how long under it, the first line of its answer in a speech
// bubble; a click opens it in Agents, a hover or a long press tells its state in words. A grid that fills the screen,
// or office mode's one large beside a column of small ones; the pop-out shows one large.

import { useEffect, useMemo, useRef, useState, type CSSProperties } from "react";
import { displayOf, rowLabel, stateWords, type RadarPrefs } from "@/features/roster/radar";
import type { Tile, WallPrefs } from "@/features/roster/wall";
import { folderOf as sessionFolder } from "@/features/roster/workspaces";
import { CREATURE_SIZE, bubbleOf, cellMin, lookOf, poseWord, stateSince } from "@/features/roster/zoo";
import { basename } from "@/lib/text";
import { elapsed } from "@/lib/time";
import { FolderIcon, Logo, Tip } from "@/ui";
import { brandColor } from "@/ui/brand";
import { longPress } from "@/ui/press";
import { Creature } from "./Creature";
import { actions } from "./store";
import type { Office } from "./useOffice";
import { t } from "@/lib/i18n";

// PEEK_MS is how long a long press keeps the state in words under a creature
const PEEK_MS = 3000;

export function Zoo({ shown, size, radar, now, office }: { shown: Tile[]; size: WallPrefs["size"]; radar: RadarPrefs; now: number; office: Office | null }) {
  const featured = office ? (shown.find((tile) => tile.key === office.key) ?? null) : null;
  if (office && featured) {
    return (
      <div className="wall-office zoo-office">
        <div className="wall-office-main zoo-featured">
          <ZooCreature tile={featured} radar={radar} now={now} mode="big" />
        </div>
        <div className="wall-office-cards zoo-cards" aria-label={t("The other agents; a click pins one")}>
          {shown
            .filter((tile) => tile !== featured)
            .map((tile) => (
              <ZooCreature key={tile.key} tile={tile} radar={radar} now={now} mode="small" pinned={tile.key === office.pinned} onPick={() => office.pin(tile.key)} />
            ))}
        </div>
      </div>
    );
  }
  // the bubbles' row is kept in the grid only while some creature talks, so a quiet zoo has no empty band
  const talking = shown.some((tile) => bubbleOf(tile.pane, displayOf(tile.pane, radar, now), tile.pane.radar?.alert) !== "");
  return (
    <div className="zoo-grid" style={{ "--zoo-cell": `${cellMin(size)}px`, "--zoo-size": `${CREATURE_SIZE[size]}px` } as CSSProperties}>
      {shown.map((tile) => (
        <ZooCreature key={tile.key} tile={tile} radar={radar} now={now} bubbleRow={talking} />
      ))}
    </div>
  );
}

// ZooCreature is one agent of the zoo: in the grid with room for its bubble, large (office mode's featured one, the
// pop-out), or small beside its caption (office mode's column, where a click pins it)
export function ZooCreature({ tile, radar, now, mode = "grid", bubbleRow = false, pinned = false, onPick }: {
  tile: Tile;
  radar: RadarPrefs;
  now: number;
  mode?: "grid" | "big" | "small";
  bubbleRow?: boolean; // the grid keeps a bubble's room over every creature, so the row stands in a line
  pinned?: boolean;
  onPick?: () => void;
}) {
  const { machine, session, tab, pane } = tile;
  const display = displayOf(pane, radar, now);
  const alert = pane.radar?.alert;
  const look = lookOf(display, alert);
  const words = alert ?? stateWords(display, pane.radar?.last_active, now, pane.radar?.since);
  const since = stateSince(pane);
  const how = since ? `${poseWord(look.pose)} · ${elapsed(since, now)}` : poseWord(look.pose);
  const bubble = mode === "small" ? "" : bubbleOf(pane, display, alert);
  const name = rowLabel(pane, tab, session.name, radar);
  const folder = basename(sessionFolder(session)) || session.name;
  const where = `${folder}${machine.id !== "local" ? ` · ${machine.label}` : ""}`;
  const [peek, setPeek] = useState(false);
  const timer = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  useEffect(() => () => clearTimeout(timer.current), []);
  const press = useMemo(
    () =>
      longPress(() => {
        setPeek(true);
        clearTimeout(timer.current);
        timer.current = setTimeout(() => setPeek(false), PEEK_MS);
      }),
    [],
  );
  const open = onPick ?? (() => actions.openPane(machine.id, pane.id));
  const action = onPick ? t("Click to feature this agent and keep it there (pin)") : t("Click to open it in Agents");
  return (
    <figure className={`zoo-creature zoo-as-${mode} is-${look.pose}${look.dim ? " dim" : ""}${pinned ? " is-pinned" : ""}`} style={{ "--zoo-skin": brandColor(pane.agent) } as CSSProperties}>
      {(bubbleRow || bubble) && (
        <div className="zoo-bubble-slot">
          {bubble && (
            <div className="zoo-bubble">
              <span>{bubble}</span>
            </div>
          )}
        </div>
      )}
      <Tip text={`${name} — ${words}. ${action}`}>
        <button type="button" className="linkish zoo-open" aria-label={`${name}: ${words}`} aria-pressed={onPick ? pinned : undefined} onClick={open} {...press}>
          <Creature look={look} />
          <span className="zoo-badge">
            <Logo agent={pane.agent} />
          </span>
        </button>
      </Tip>
      <figcaption>
        <span className="zoo-name">{name}</span>
        <span className="zoo-where">
          <FolderIcon icon={session.icon} name={folder} size={12} />
          <span>{where}</span>
        </span>
        <span className="zoo-how">{peek ? words : how}</span>
      </figcaption>
    </figure>
  );
}
