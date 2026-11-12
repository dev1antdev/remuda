// What a zoo creature wears, in its own 120 × 128 units: the hat of its model on the top of its head (near y 34), a
// crown once its pull request is merged, and its trinket — glasses, a bow, a scarf, a bow tie, freckles or a mustache
// (spots are on the skin, in bodies.tsx).

import type { Hat as HatKind, Trinket as TrinketKind } from "@/features/roster/zooWear";
import { sparkle } from "./bodies";

export function Hat({ hat }: { hat: HatKind }) {
  switch (hat) {
    case "tophat":
      return (
        <g className="cr-hat is-tophat">
          <rect className="cr-hat-main" x="46" y="12" width="28" height="22" rx="2.5" />
          <rect className="cr-hat-band" x="46" y="27" width="28" height="4.5" />
          <ellipse className="cr-hat-main" cx="60" cy="34.5" rx="19" ry="3.6" />
        </g>
      );
    case "wizard":
      return (
        <g className="cr-hat is-wizard">
          <path className="cr-hat-main" d="M43 35 L58 4 Q61 -1 63 5 L77 35 Z" />
          <ellipse className="cr-hat-main" cx="60" cy="35" rx="21" ry="4" />
          <path className="cr-hat-star" d={sparkle(57, 22, 4.2)} />
          <path className="cr-hat-star" d={sparkle(66, 13, 2.8)} />
        </g>
      );
    case "beret":
      return (
        <g className="cr-hat is-beret">
          <ellipse className="cr-hat-main" cx="57" cy="33" rx="20" ry="6.5" transform="rotate(-9 57 33)" />
          <path className="cr-hat-stalk" d="M56 27 Q56 23 59 21" />
        </g>
      );
    case "sprout":
      return (
        <g className="cr-hat is-sprout">
          <path className="cr-hat-stalk" d="M60 35 Q59 28 61 21" />
          <path className="cr-hat-main" d="M60 27 Q50 18 44 24 Q51 31 60 27 Z" />
          <path className="cr-hat-main" d="M61 23 Q70 12 77 18 Q70 26 61 23 Z" />
        </g>
      );
    case "laurel":
      return (
        <g className="cr-hat is-laurel">
          <path className="cr-hat-stalk" d="M38 48 Q42 36 56 32" />
          <path className="cr-hat-stalk" d="M82 48 Q78 36 64 32" />
          <ellipse className="cr-hat-main" cx="41" cy="41" rx="5" ry="2.6" transform="rotate(-50 41 41)" />
          <ellipse className="cr-hat-main" cx="47" cy="35" rx="5" ry="2.6" transform="rotate(-30 47 35)" />
          <ellipse className="cr-hat-main" cx="54" cy="31.5" rx="4.6" ry="2.4" transform="rotate(-10 54 31.5)" />
          <ellipse className="cr-hat-main" cx="79" cy="41" rx="5" ry="2.6" transform="rotate(50 79 41)" />
          <ellipse className="cr-hat-main" cx="73" cy="35" rx="5" ry="2.6" transform="rotate(30 73 35)" />
          <ellipse className="cr-hat-main" cx="66" cy="31.5" rx="4.6" ry="2.4" transform="rotate(10 66 31.5)" />
        </g>
      );
    case "cap":
      return (
        <g className="cr-hat is-cap">
          <path className="cr-hat-main" d="M41 36 Q41 18 60 18 Q79 18 79 36 Z" />
          <path className="cr-hat-visor" d="M70 33 Q88 30 95 36 Q82 40 70 38 Z" />
          <circle className="cr-hat-band" cx="60" cy="18.5" r="2.4" />
        </g>
      );
    case "crown":
      return (
        <g className="cr-hat is-crown">
          <path className="cr-hat-main" d="M43 36 L43 19 L51.5 27 L60 13 L68.5 27 L77 19 L77 36 Z" />
          <circle className="cr-gem" cx="60" cy="30" r="3" />
          <circle className="cr-gem is-side" cx="50" cy="32" r="2" />
          <circle className="cr-gem is-side" cx="70" cy="32" r="2" />
        </g>
      );
    default:
      return null;
  }
}

export function Trinket({ trinket }: { trinket: TrinketKind }) {
  switch (trinket) {
    case "glasses":
      return (
        <g className="cr-glasses">
          <circle cx="48" cy="62" r="10" />
          <circle cx="72" cy="62" r="10" />
          <path d="M58 61 Q60 59 62 61 M38 60 L30 57 M82 60 L90 57" />
        </g>
      );
    case "bow":
      return (
        <g className="cr-bow">
          <path d="M34 45 L23 38 L24 52 Z" />
          <path d="M34 45 L45 38 L44 52 Z" />
          <circle cx="34" cy="45" r="3" />
        </g>
      );
    case "scarf":
      return (
        <g className="cr-scarf">
          <path d="M30 84 Q60 95 90 84 L91 91 Q60 102 29 91 Z" />
          <path d="M72 92 L79 108 L70 108 Z" />
        </g>
      );
    case "bowtie":
      return (
        <g className="cr-bowtie">
          <path d="M60 89 L49 83 L49 95 Z" />
          <path d="M60 89 L71 83 L71 95 Z" />
          <circle cx="60" cy="89" r="2.6" />
        </g>
      );
    case "freckles":
      return (
        <g className="cr-freckles">
          <circle cx="35" cy="70" r="1.2" />
          <circle cx="39.5" cy="74" r="1.2" />
          <circle cx="34" cy="76" r="1.2" />
          <circle cx="85" cy="70" r="1.2" />
          <circle cx="80.5" cy="74" r="1.2" />
          <circle cx="86" cy="76" r="1.2" />
        </g>
      );
    case "mustache":
      return <path className="cr-mustache" d="M60 74 Q52 69 45 74.5 Q52 77.5 60 75.5 Q68 77.5 75 74.5 Q68 69 60 74 Z" />;
    default:
      return null;
  }
}
