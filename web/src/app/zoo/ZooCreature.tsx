// ZooCreature is one agent of the zoo: the creature (./Creature) dressed off the roster, the speech bubble over it — the
// first line of its answer, or what it asks while it asks — and its name, folder and how long under it; a click opens
// it in Agents, a hover or a long press tells its state in words. In the grid with room for its bubble, large (office
// mode's featured one, the pop-out), or small beside its caption (office mode's column, where a click pins it).

import { useEffect, useMemo, useRef, useState, type CSSProperties } from "react";
import { displayOf, rowLabel, stateWords, type RadarPrefs } from "@/features/roster/radar";
import { useRoster } from "@/features/roster/useRoster";
import { useSharing } from "@/features/roster/useSharing";
import type { Tile } from "@/features/roster/wall";
import { folderOf as sessionFolder } from "@/features/roster/workspaces";
import { bubbleOf, lookOf, poseWord, stateSince, zooFace } from "@/features/roster/zoo";
import { BELT_AT, babiesOf, broodWords, coinsOf, drainedUntil, flagOf, flagWords, girthOf, workExtras } from "@/features/roster/zooBody";
import { weatherOf } from "@/features/roster/zooPens";
import { outfits, wearersOf, type Outfit } from "@/features/roster/zooWear";
import { budgetTone, budgetsFor, ratio, tightest } from "@/features/usage/budgets";
import { usd } from "@/features/usage/format";
import { t } from "@/lib/i18n";
import { basename } from "@/lib/text";
import { elapsed, whenAhead } from "@/lib/time";
import { FolderIcon, Logo, Tip } from "@/ui";
import { VENDOR_NAME, brandColor } from "@/ui/brand";
import { longPress } from "@/ui/press";
import { actions } from "../store";
import { Creature, type Dress } from "./Creature";

// PEEK_MS is how long a long press keeps the state in words under a creature
const PEEK_MS = 3000;

// Life is what the zoo around a creature gives it: its outfit among the others, the applause, a puff, what it asks
export type Life = { outfit?: Outfit; cheer?: boolean; puff?: boolean; question?: string; night?: boolean };

export function ZooCreature({ tile, radar, now, mode = "grid", bubbleRow = false, pinned = false, onPick, life = {} }: {
  tile: Tile;
  radar: RadarPrefs;
  now: number;
  mode?: "grid" | "big" | "small";
  bubbleRow?: boolean; // the grid keeps a bubble's room over every creature, so the row stands in a line
  pinned?: boolean;
  onPick?: () => void;
  life?: Life;
}) {
  const { machine, session, tab, pane } = tile;
  const { budgets, machines: roster } = useRoster();
  const share = useSharing();
  const hidden = share.sessionHidden(machine.id, session);
  const display = displayOf(pane, radar, now);
  const alert = pane.radar?.alert;
  const look = lookOf(display, alert, !!pane.limit?.hit_at);
  const until = look.pose === "drained" ? drainedUntil(pane.limit, machine.limits?.windows ?? [], pane.agent) : undefined;
  const words = alert ?? (look.pose === "drained" ? poseWord("drained") : stateWords(display, pane.radar?.last_active, now, pane.radar?.since));
  const since = stateSince(pane);
  const how = until ? `${poseWord(look.pose)} · ${t("until {time}", { time: whenAhead(until) })}` : since ? `${poseWord(look.pose)} · ${elapsed(since, now)}` : poseWord(look.pose);
  const realFolder = basename(sessionFolder(session)) || session.name;
  const folder = hidden ? share.folderName(machine.id, sessionFolder(session), realFolder) : realFolder;
  const asking = look.pose === "asking" && !!life.question && !hidden && mode !== "small";
  const { name, where, bubble, icon } = zooFace(
    {
      label: rowLabel(pane, tab, session.name, radar),
      kind: VENDOR_NAME[pane.agent] ?? pane.agent,
      folder,
      machine: machine.id !== "local" ? share.machineLabel(machine) : null,
      bubble: mode === "small" ? "" : asking ? life.question! : bubbleOf(pane, display, alert),
    },
    hidden,
  );
  const dress = useMemo<Dress>(() => {
    const outfit = life.outfit ?? outfits(wearersOf(roster)).get(tile.key) ?? outfits([{ key: tile.key, agent: pane.agent, model: pane.usage?.model }]).get(tile.key)!;
    const girth = girthOf(pane.usage);
    const budget = tightest(budgetsFor(budgets, machine.id, session.dir || pane.cwd));
    const tone = budget ? budgetTone(ratio(budget)) : "";
    const { babies, more } = babiesOf(pane.subagents);
    const extras = workExtras(display, since, now, weatherOf(machine.health, machine.awake).heat === "hot");
    return {
      outfit,
      girth,
      belt: girth >= BELT_AT,
      coins: coinsOf(pane.usage?.cost),
      coinTone: tone === "amber" || tone === "red" ? tone : "",
      flag: hidden ? "" : flagOf(pane.pr),
      ...extras,
      babies,
      more,
      cheer: !!life.cheer,
      puff: !!life.puff,
      night: !!life.night,
    };
  }, [life.outfit, life.cheer, life.puff, life.night, tile.key, pane, budgets, roster, machine, session.dir, display, since, now, hidden]);
  const extras = [
    pane.usage?.name,
    pane.usage?.cost ? usd(pane.usage.cost) : "",
    pane.usage && pane.usage.window > 0 ? t("{pct}% context", { pct: Math.round(100 * girthOf(pane.usage)) }) : "",
    hidden || !pane.pr ? "" : flagWords(flagOf(pane.pr), pane.pr.label),
    broodWords(pane.subagents),
  ].filter(Boolean);
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
  const tip = `${name} — ${words}.${extras.length ? ` ${extras.join(" · ")}.` : ""} ${action}`;
  const classes = `zoo-creature zoo-as-${mode} is-${look.pose}${look.dim ? " dim" : ""}${pinned ? " is-pinned" : ""}${dress.cheer ? " is-cheering" : ""}`;
  return (
    <figure className={classes} style={{ "--zoo-skin": brandColor(pane.agent) } as CSSProperties}>
      {(bubbleRow || bubble) && (
        <div className="zoo-bubble-slot">
          {bubble && (
            <div className={`zoo-bubble${asking ? " is-question" : ""}`}>
              <span>{bubble}</span>
            </div>
          )}
        </div>
      )}
      <Tip text={tip}>
        <button type="button" className="linkish zoo-open" aria-label={`${name}: ${words}`} aria-pressed={onPick ? pinned : undefined} onClick={open} {...press}>
          <Creature look={look} dress={dress} />
          <span className="zoo-badge">
            <Logo agent={pane.agent} />
          </span>
        </button>
      </Tip>
      <figcaption>
        <span className="zoo-name">{name}</span>
        <span className="zoo-where">
          <FolderIcon icon={icon ? session.icon : undefined} name={folder} size={12} />
          <span>{where}</span>
        </span>
        <span className="zoo-how">{peek ? words : how}</span>
      </figcaption>
    </figure>
  );
}
