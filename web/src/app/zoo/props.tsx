// What a zoo creature holds and what stands around it, in its own 120 × 128 units: the laptop and the mug of a long
// stretch of work, confetti, the sign over its head (top right, clear of the hats), the pile of coins its conversation
// cost (bottom left), its pull request's flag (left), sweat, a compaction's puff, a blanket at night, and its subagents
// as babies (bottom right).

import type { Look } from "@/features/roster/zoo";
import type { Flag as FlagKind } from "@/features/roster/zooBody";
import type { Species } from "@/features/roster/zooWear";
import { skinOf } from "./bodies";

export function Laptop() {
  return (
    <g className="cr-laptop">
      <path className="cr-screen" d="M38 104 L42 85 L78 85 L82 104 Z" />
      <rect className="cr-code" x="47" y="91" width="14" height="2.4" rx="1.2" />
      <rect className="cr-code cr-code-2" x="47" y="96" width="22" height="2.4" rx="1.2" />
      <rect className="cr-base" x="33" y="103" width="54" height="6" rx="2.5" />
    </g>
  );
}

export function Coffee() {
  return (
    <g className="cr-coffee">
      <path className="cr-steam" d="M93 87 Q91 83 93 80 Q95 77 93 74" />
      <path className="cr-steam cr-steam-2" d="M98 87 Q96 83 98 80 Q100 77 98 74" />
      <path className="cr-mug-handle" d="M102 93 Q108 94.5 107 98 Q106 101 102 100.5" />
      <rect className="cr-mug" x="89" y="90" width="13" height="13" rx="2.6" />
    </g>
  );
}

export function Confetti() {
  return (
    <g className="cr-confetti">
      <rect className="cr-bit is-yellow" x="14" y="26" width="5" height="5" rx="1" />
      <circle className="cr-bit is-pink" cx="106" cy="30" r="3" />
      <rect className="cr-bit is-sky" x="8" y="58" width="4" height="6" rx="1" />
      <circle className="cr-bit is-green" cx="110" cy="62" r="2.6" />
      <rect className="cr-bit is-mauve" x="58" y="4" width="5" height="5" rx="1" />
    </g>
  );
}

export function Sign({ sign }: { sign: Look["sign"] }) {
  switch (sign) {
    case "?":
      return (
        <text className="cr-sign is-ask" x="101" y="26" fontSize="24">
          ?
        </text>
      );
    case "!":
      return (
        <g className="cr-sign is-note">
          <rect className="cr-board" x="90" y="2" width="20" height="22" rx="4" />
          <text className="cr-board-mark" x="100" y="19" fontSize="17">
            !
          </text>
        </g>
      );
    case "z":
      return (
        <g className="cr-sleep">
          <text className="cr-z" x="86" y="34" fontSize="12">
            z
          </text>
          <text className="cr-z" x="95" y="23" fontSize="15">
            z
          </text>
          <text className="cr-z" x="105" y="12" fontSize="18">
            Z
          </text>
        </g>
      );
    case "…":
      return (
        <text className="cr-sign is-puzzled" x="101" y="24" fontSize="22">
          …
        </text>
      );
    case "battery":
      return (
        <g className="cr-sign is-battery">
          <rect className="cr-battery" x="88" y="8" width="22" height="12" rx="2.5" />
          <rect className="cr-battery-tip" x="110" y="11.5" width="3" height="5" rx="1" />
          <rect className="cr-battery-left" x="90.5" y="10.5" width="3.5" height="7" rx="1" />
        </g>
      );
    default:
      return null;
  }
}

// Coins is the conversation's cost as a pile, tinted when a budget over it runs short
export function Coins({ count, tone }: { count: number; tone: "" | "amber" | "red" }) {
  if (!count) return null;
  return (
    <g className={`cr-coins${tone ? ` is-${tone}` : ""}`}>
      {Array.from({ length: count }, (_, i) => (
        <g key={i}>
          <ellipse className="cr-coin-edge" cx="13" cy={124.6 - i * 3.6} rx="9" ry="3.2" />
          <ellipse className="cr-coin" cx="13" cy={123.6 - i * 3.6} rx="9" ry="3.2" />
        </g>
      ))}
    </g>
  );
}

export function Flag({ flag }: { flag: FlagKind }) {
  if (!flag || flag === "merged") return null;
  return (
    <g className={`cr-flag is-${flag}`}>
      <path className="cr-pole" d="M8 118 L8 52" />
      <path className="cr-cloth" d="M8 52 L29 58 L8 64 Z" />
    </g>
  );
}

export function Sweat() {
  return (
    <g className="cr-sweat">
      <path d="M27 44 Q31 50 27 53.5 Q23 50 27 44 Z" />
      <path className="cr-drop-2" d="M94 40 Q98 46 94 49.5 Q90 46 94 40 Z" />
    </g>
  );
}

export function Puff() {
  return (
    <g className="cr-puff">
      <circle cx="40" cy="44" r="7" />
      <circle cx="60" cy="34" r="9" />
      <circle cx="80" cy="44" r="7" />
      <circle cx="30" cy="62" r="5" />
      <circle cx="90" cy="62" r="5" />
    </g>
  );
}

export function Blanket() {
  return (
    <g className="cr-blanket">
      <path className="cr-blanket-cloth" d="M20 92 Q60 82 100 92 L102 116 Q60 123 18 116 Z" />
      <path className="cr-blanket-hem" d="M21 97 Q60 88 99 97" />
    </g>
  );
}

const BABY_X = [91, 103, 115];
const BABY_SCALE = 0.2;

// Babies are the conversation's subagents at work, at the creature's feet, each at a little laptop of its own
export function Babies({ species, babies, more }: { species: Species; babies: number; more: number }) {
  if (!babies) return null;
  return (
    <g className="cr-babies">
      {BABY_X.slice(0, babies).map((x) => (
        <g key={x} transform={`translate(${x - 60 * BABY_SCALE} ${127 - 112 * BABY_SCALE}) scale(${BABY_SCALE})`}>
          <g className="cr-baby">
            <path className="cr-skin" d={skinOf(species)} />
            <circle className="cr-pupil" cx="49" cy="66" r="6" />
            <circle className="cr-pupil" cx="71" cy="66" r="6" />
            <rect className="cr-screen" x="36" y="84" width="48" height="22" rx="4" />
          </g>
        </g>
      ))}
      {more > 0 && (
        <text className="cr-babies-more" x="116" y="104" fontSize="9">
          +{more}
        </text>
      )}
    </g>
  );
}
