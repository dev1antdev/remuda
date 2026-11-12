// Creature is one zoo animal drawn in SVG, its 120 × 128 units shared by every part: the body of its species in the
// vendor's hue (--zoo-skin, the rest of its palette mixed from it and the flavour's tokens in styles.css), widened by its
// context (--zoo-girth); a pose — face, arms, a laptop, confetti, the sign over its head — for each state; its model's
// hat and its trinket; the coins, the flag, the mug, the sweat, the blanket and the babies its state brings. The motion
// is CSS, keyed on the classes, and stops for reduced motion.

import type { Look } from "@/features/roster/zoo";
import type { Flag as FlagKind } from "@/features/roster/zooBody";
import type { Outfit } from "@/features/roster/zooWear";
import type { CSSProperties } from "react";
import { Body } from "./bodies";
import { Arms, Face } from "./face";
import { Babies, Blanket, Coffee, Coins, Confetti, Flag, Laptop, Puff, Sign, Sweat } from "./props";
import { Hat, Trinket } from "./wear";

// Dress is everything a creature shows beside its pose
export type Dress = {
  outfit: Outfit;
  girth: number; // 0 to 1, how full its context is
  belt: boolean;
  coins: number;
  coinTone: "" | "amber" | "red";
  flag: FlagKind;
  coffee: boolean;
  sweat: boolean;
  babies: number; // its subagents at work, drawn
  more: number; // and the ones past them
  cheer: boolean; // clapping for another's finish
  puff: boolean; // just compacted
  night: boolean;
};

export function Creature({ look, dress }: { look: Look; dress: Dress }) {
  const { pose, sign } = look;
  const { outfit } = dress;
  const busy = pose === "busy" && !dress.cheer;
  const hat = dress.flag === "merged" ? "crown" : outfit.hat;
  const style = { "--zoo-girth": dress.girth, "--zoo-blink": `${-(outfit.seed % 5000) / 1000}s` } as CSSProperties;
  return (
    <svg className={`creature is-${outfit.species}${outfit.small ? " is-small" : ""}`} viewBox="0 0 120 128" aria-hidden="true" style={style}>
      <ellipse className="cr-shadow" cx="60" cy="121" rx={34 + 8 * dress.girth} ry="4.5" />
      {pose === "dancing" && <Confetti />}
      <Flag flag={dress.flag} />
      <g className="cr-frame">
        <g className="cr-body">
          <Body species={outfit.species} trinket={outfit.trinket} belt={dress.belt} />
          <Face pose={pose} cheer={dress.cheer} />
          <Trinket trinket={outfit.trinket} />
          <Hat hat={hat} />
          {!busy && <Arms pose={pose} cheer={dress.cheer} />}
          {dress.night && pose === "asleep" && !dress.cheer && <Blanket />}
          {dress.sweat && <Sweat />}
          {dress.puff && <Puff />}
        </g>
        {busy && <Laptop />}
        {busy && dress.coffee && <Coffee />}
        {busy && <Arms pose={pose} cheer={false} />}
      </g>
      <Coins count={dress.coins} tone={dress.coinTone} />
      <Babies species={outfit.species} babies={dress.babies} more={dress.more} />
      {!dress.cheer && <Sign sign={sign} />}
    </svg>
  );
}
