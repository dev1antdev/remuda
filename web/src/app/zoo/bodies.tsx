// The zoo's species, one body each in the creature's own 120 × 128 units: Claude Code's round critter with antennae,
// Codex's boxy robot, Gemini CLI's egg between two sparkles, Kimi Code's moon rabbit, a tufted blob for any other agent.
// The skin and belly sit in .cr-fat, which the context's girth widens; the head's top stays near y 34 for the hats.

import type { Species, Trinket } from "@/features/roster/zooWear";

const ROUND = "M60 34 C86 34 96 54 96 76 C96 100 82 112 60 112 C38 112 24 100 24 76 C24 54 34 34 60 34 Z";
const BOX = "M39 36 H81 Q96 36 96 51 V99 Q96 112 81 112 H39 Q24 112 24 99 V51 Q24 36 39 36 Z";
const EGG = "M60 28 C72 40 96 54 96 78 C96 102 82 112 60 112 C38 112 24 102 24 78 C24 54 48 40 60 28 Z";

// skinOf is a species' outline, which the babies wear too
export const skinOf = (species: Species): string => (species === "codex" ? BOX : species === "gemini" ? EGG : ROUND);

// sparkle is a four-pointed star with hollow sides around a centre
export const sparkle = (cx: number, cy: number, r: number) =>
  `M${cx} ${cy - r} Q${cx} ${cy} ${cx + r} ${cy} Q${cx} ${cy} ${cx} ${cy + r} Q${cx} ${cy} ${cx - r} ${cy} Q${cx} ${cy} ${cx} ${cy - r} Z`;

// Body is everything of a creature but its face, arms and what it wears or holds; belt is the context's, from 80 %
export function Body({ species, trinket, belt }: { species: Species; trinket: Trinket; belt: boolean }) {
  return (
    <>
      <Feet species={species} />
      <Crest species={species} />
      <g className="cr-fat">
        <path className="cr-skin" d={skinOf(species)} />
        <Belly species={species} />
        {trinket === "spots" && (
          <g className="cr-spots">
            <ellipse cx="35" cy="86" rx="4.4" ry="3.6" />
            <ellipse cx="85" cy="62" rx="3.6" ry="3" />
            <ellipse cx="39" cy="55" rx="2.6" ry="2.2" />
            <ellipse cx="86" cy="92" rx="2.4" ry="2" />
          </g>
        )}
        {belt && (
          <g className="cr-belt">
            <rect className="cr-strap" x="25" y="88" width="70" height="5.5" rx="1.5" />
            <rect className="cr-buckle" x="55.5" y="86.6" width="9" height="8.3" rx="1.6" />
          </g>
        )}
      </g>
      {species === "gemini" && (
        <g className="cr-sparkles">
          <path className="cr-sparkle" d={sparkle(27, 34, 7)} />
          <path className="cr-sparkle cr-sparkle-2" d={sparkle(94, 28, 9)} />
        </g>
      )}
    </>
  );
}

function Feet({ species }: { species: Species }) {
  if (species === "codex")
    return (
      <>
        <rect className="cr-foot" x="36" y="107" width="21" height="11" rx="3.5" />
        <rect className="cr-foot" x="63" y="107" width="21" height="11" rx="3.5" />
      </>
    );
  return (
    <>
      <ellipse className="cr-foot" cx="47" cy="113" rx="10" ry="5.5" />
      <ellipse className="cr-foot" cx="73" cy="113" rx="10" ry="5.5" />
    </>
  );
}

// Crest is what stands up off the head, behind the skin: antennae, a robot's aerial and bolts, ears, a tuft
function Crest({ species }: { species: Species }) {
  switch (species) {
    case "claude":
      return (
        <>
          <path className="cr-antenna" d="M48 38 Q44 26 38 22" />
          <path className="cr-antenna" d="M72 38 Q76 26 82 22" />
          <circle className="cr-tip" cx="37" cy="21" r="4.5" />
          <circle className="cr-tip" cx="83" cy="21" r="4.5" />
        </>
      );
    case "codex":
      return (
        <>
          <path className="cr-antenna" d="M76 38 L82 20" />
          <circle className="cr-tip cr-bulb" cx="83" cy="17" r="4.6" />
          <rect className="cr-foot" x="18" y="62" width="8" height="14" rx="2.5" />
          <rect className="cr-foot" x="94" y="62" width="8" height="14" rx="2.5" />
        </>
      );
    case "kimi":
      return (
        <>
          <ellipse className="cr-ear" cx="45" cy="21" rx="7.5" ry="19" transform="rotate(-14 45 21)" />
          <ellipse className="cr-ear" cx="75" cy="21" rx="7.5" ry="19" transform="rotate(14 75 21)" />
          <ellipse className="cr-ear-in" cx="45" cy="22" rx="3.4" ry="13" transform="rotate(-14 45 22)" />
          <ellipse className="cr-ear-in" cx="75" cy="22" rx="3.4" ry="13" transform="rotate(14 75 22)" />
        </>
      );
    case "other":
      return (
        <>
          <path className="cr-antenna" d="M60 36 Q58 26 54 22" />
          <path className="cr-antenna" d="M60 36 Q61 25 64 20" />
          <path className="cr-antenna" d="M61 36 Q66 28 71 26" />
        </>
      );
    default:
      return null;
  }
}

function Belly({ species }: { species: Species }) {
  if (species === "codex")
    return (
      <g className="cr-panel">
        <rect className="cr-belly" x="42" y="87" width="36" height="17" rx="4" />
        <circle className="cr-led" cx="51" cy="95.5" r="2.4" />
        <circle className="cr-led cr-led-2" cx="60" cy="95.5" r="2.4" />
        <circle className="cr-led cr-led-3" cx="69" cy="95.5" r="2.4" />
      </g>
    );
  return (
    <>
      <ellipse className="cr-belly" cx="60" cy="97" rx="20" ry="11" />
      {species === "kimi" && <path className="cr-moon" d="M62 90 A7 7 0 1 0 62 104 A5.4 5.4 0 1 1 62 90 Z" />}
    </>
  );
}
