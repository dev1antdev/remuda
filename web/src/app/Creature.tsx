// Creature is one zoo animal drawn in SVG: a round body in the vendor's hue (--zoo-skin, the rest of its palette mixed
// from it and the flavour's tokens in styles.css), and a pose — arms, eyes, mouth, a laptop or confetti, a sign over its
// head — for each state; the motion is CSS, keyed on the pose's class, and stops for reduced motion.

import type { Look } from "@/features/roster/zoo";

const BODY = "M60 34 C86 34 96 54 96 76 C96 100 82 112 60 112 C38 112 24 100 24 76 C24 54 34 34 60 34 Z";

export function Creature({ look }: { look: Look }) {
  const { pose, sign } = look;
  return (
    <svg className="creature" viewBox="0 0 120 128" aria-hidden="true">
      <ellipse className="cr-shadow" cx="60" cy="121" rx="34" ry="4.5" />
      {pose === "dancing" && <Confetti />}
      <g className="cr-body">
        <ellipse className="cr-foot" cx="47" cy="113" rx="10" ry="5.5" />
        <ellipse className="cr-foot" cx="73" cy="113" rx="10" ry="5.5" />
        <path className="cr-antenna" d="M48 38 Q44 26 38 22" />
        <path className="cr-antenna" d="M72 38 Q76 26 82 22" />
        <circle className="cr-tip" cx="37" cy="21" r="4.5" />
        <circle className="cr-tip" cx="83" cy="21" r="4.5" />
        <path className="cr-skin" d={BODY} />
        <ellipse className="cr-belly" cx="60" cy="97" rx="20" ry="11" />
        <Face pose={pose} />
        {pose !== "busy" && <Arms pose={pose} />}
      </g>
      {pose === "busy" && <Laptop />}
      {pose === "busy" && <Arms pose={pose} />}
      <Sign sign={sign} />
    </svg>
  );
}

function Arms({ pose }: { pose: Look["pose"] }) {
  switch (pose) {
    case "busy":
      return (
        <g className="cr-arms">
          <path className="cr-arm cr-arm-l" d="M32 82 Q34 96 46 101" />
          <path className="cr-arm cr-arm-r" d="M88 82 Q86 96 74 101" />
        </g>
      );
    case "asking":
      return (
        <g className="cr-arms">
          <path className="cr-arm" d="M30 80 Q20 88 24 99" />
          <g className="cr-arm-r">
            <path className="cr-arm" d="M90 74 Q104 60 100 42" />
            <circle className="cr-hand" cx="100" cy="39" r="6" />
          </g>
        </g>
      );
    case "dancing":
      return (
        <g className="cr-arms">
          <g className="cr-arm-l">
            <path className="cr-arm" d="M30 74 Q16 60 20 44" />
            <circle className="cr-hand" cx="20" cy="41" r="5.5" />
          </g>
          <g className="cr-arm-r">
            <path className="cr-arm" d="M90 74 Q104 60 100 44" />
            <circle className="cr-hand" cx="100" cy="41" r="5.5" />
          </g>
        </g>
      );
    case "worried":
      return (
        <g className="cr-arms">
          <path className="cr-arm" d="M30 82 Q22 74 34 70" />
          <path className="cr-arm" d="M90 82 Q98 74 86 70" />
        </g>
      );
    case "puzzled":
      return (
        <g className="cr-arms">
          <path className="cr-arm" d="M30 80 Q20 88 24 99" />
          <path className="cr-arm" d="M90 76 Q104 62 90 48" />
          <circle className="cr-hand" cx="88" cy="46" r="5.5" />
        </g>
      );
    default:
      return (
        <g className="cr-arms">
          <path className="cr-arm" d="M30 80 Q20 88 24 99" />
          <path className="cr-arm" d="M90 80 Q100 88 96 99" />
        </g>
      );
  }
}

function Eye({ cx, cy, dx = 0, dy = 0, r = 8 }: { cx: number; cy: number; dx?: number; dy?: number; r?: number }) {
  return (
    <g>
      <circle className="cr-white" cx={cx} cy={cy} r={r} />
      <circle className="cr-pupil" cx={cx + dx} cy={cy + dy} r={r / 2} />
      <circle className="cr-glint" cx={cx + dx - r / 5} cy={cy + dy - r / 5} r={r / 5} />
    </g>
  );
}

function Face({ pose }: { pose: Look["pose"] }) {
  switch (pose) {
    case "busy":
      return (
        <g className="cr-face">
          <ellipse className="cr-white" cx="48" cy="63" rx="8" ry="5" />
          <ellipse className="cr-white" cx="72" cy="63" rx="8" ry="5" />
          <circle className="cr-pupil" cx="49" cy="65" r="3.4" />
          <circle className="cr-pupil" cx="71" cy="65" r="3.4" />
          <path className="cr-line" d="M55 79 L65 79" />
        </g>
      );
    case "asking":
      return (
        <g className="cr-face">
          <Eye cx={48} cy={62} dy={-3} />
          <Eye cx={72} cy={62} dx={1} dy={-3} />
          <circle className="cr-mouth" cx="60" cy="79" r="3.6" />
        </g>
      );
    case "worried":
      return (
        <g className="cr-face">
          <path className="cr-line" d="M39 52 L54 48" />
          <path className="cr-line" d="M81 52 L66 48" />
          <Eye cx={48} cy={62} />
          <Eye cx={72} cy={62} />
          <path className="cr-line" d="M51 80 Q55.5 76 60 80 Q64.5 84 69 80" />
        </g>
      );
    case "dancing":
      return (
        <g className="cr-face">
          <path className="cr-line" d="M41 64 Q48 55 55 64" />
          <path className="cr-line" d="M65 64 Q72 55 79 64" />
          <path className="cr-mouth" d="M50 75 Q60 88 70 75 Z" />
          <ellipse className="cr-cheek" cx="38" cy="73" rx="4.5" ry="2.8" />
          <ellipse className="cr-cheek" cx="82" cy="73" rx="4.5" ry="2.8" />
        </g>
      );
    case "puzzled":
      return (
        <g className="cr-face">
          <path className="cr-line" d="M65 50 Q72 45 80 50" />
          <Eye cx={48} cy={62} dx={2} />
          <Eye cx={72} cy={63} r={5.5} dx={1} />
          <path className="cr-line" d="M54 81 L66 78" />
        </g>
      );
    default:
      return (
        <g className="cr-face">
          <path className="cr-line" d="M41 62 Q48 67 55 62" />
          <path className="cr-line" d="M65 62 Q72 67 79 62" />
          <path className="cr-line" d="M56 79 Q60 81.5 64 79" />
          <ellipse className="cr-cheek" cx="38" cy="72" rx="4.5" ry="2.8" />
          <ellipse className="cr-cheek" cx="82" cy="72" rx="4.5" ry="2.8" />
        </g>
      );
  }
}

function Laptop() {
  return (
    <g className="cr-laptop">
      <path className="cr-screen" d="M38 104 L42 85 L78 85 L82 104 Z" />
      <rect className="cr-code" x="47" y="91" width="14" height="2.4" rx="1.2" />
      <rect className="cr-code cr-code-2" x="47" y="96" width="22" height="2.4" rx="1.2" />
      <rect className="cr-base" x="33" y="103" width="54" height="6" rx="2.5" />
    </g>
  );
}

function Confetti() {
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

function Sign({ sign }: { sign: Look["sign"] }) {
  switch (sign) {
    case "?":
      return (
        <text className="cr-sign is-ask" x="60" y="22" fontSize="24">
          ?
        </text>
      );
    case "!":
      return (
        <g className="cr-sign is-note">
          <rect className="cr-board" x="50" y="2" width="20" height="22" rx="4" />
          <text className="cr-board-mark" x="60" y="19" fontSize="17">
            !
          </text>
        </g>
      );
    case "z":
      return (
        <g className="cr-sleep">
          <text className="cr-z" x="84" y="32" fontSize="12">
            z
          </text>
          <text className="cr-z" x="93" y="21" fontSize="15">
            z
          </text>
          <text className="cr-z" x="103" y="10" fontSize="18">
            Z
          </text>
        </g>
      );
    case "…":
      return (
        <text className="cr-sign is-puzzled" x="60" y="20" fontSize="22">
          …
        </text>
      );
    default:
      return null;
  }
}
