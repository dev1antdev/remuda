// A zoo creature's face and arms for each pose, in its own 120 × 128 units. A round eye (Eye) blinks now and then and its
// pupil follows the pointer (--gx, --gy, set by useGaze); a cheering creature claps whatever pose it rested in.

import type { Pose } from "@/features/roster/zoo";

export function Arms({ pose, cheer }: { pose: Pose; cheer: boolean }) {
  if (cheer)
    return (
      <g className="cr-arms cr-clap">
        <path className="cr-arm" d="M30 80 Q36 92 52 90" />
        <path className="cr-arm" d="M90 80 Q84 92 68 90" />
        <circle className="cr-hand cr-hand-l" cx="53" cy="89" r="5.5" />
        <circle className="cr-hand cr-hand-r" cx="67" cy="89" r="5.5" />
      </g>
    );
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
    case "drained":
      return (
        <g className="cr-arms">
          <path className="cr-arm" d="M30 82 Q24 96 28 108" />
          <path className="cr-arm" d="M90 82 Q96 96 92 108" />
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
    <g className="cr-eye">
      <circle className="cr-white" cx={cx} cy={cy} r={r} />
      <g className="cr-gaze">
        <circle className="cr-pupil" cx={cx + dx} cy={cy + dy} r={r / 2} />
        <circle className="cr-glint" cx={cx + dx - r / 5} cy={cy + dy - r / 5} r={r / 5} />
      </g>
    </g>
  );
}

const HAPPY = (
  <g className="cr-face">
    <path className="cr-line" d="M41 64 Q48 55 55 64" />
    <path className="cr-line" d="M65 64 Q72 55 79 64" />
    <path className="cr-mouth" d="M50 75 Q60 88 70 75 Z" />
    <ellipse className="cr-cheek" cx="38" cy="73" rx="4.5" ry="2.8" />
    <ellipse className="cr-cheek" cx="82" cy="73" rx="4.5" ry="2.8" />
  </g>
);

export function Face({ pose, cheer }: { pose: Pose; cheer: boolean }) {
  if (cheer) return HAPPY;
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
      return HAPPY;
    case "puzzled":
      return (
        <g className="cr-face">
          <path className="cr-line" d="M65 50 Q72 45 80 50" />
          <Eye cx={48} cy={62} dx={2} />
          <Eye cx={72} cy={63} r={5.5} dx={1} />
          <path className="cr-line" d="M54 81 L66 78" />
        </g>
      );
    case "drained":
      return (
        <g className="cr-face">
          <ellipse className="cr-white" cx="48" cy="65" rx="7.5" ry="4" />
          <ellipse className="cr-white" cx="72" cy="65" rx="7.5" ry="4" />
          <circle className="cr-pupil" cx="48" cy="67" r="2.8" />
          <circle className="cr-pupil" cx="72" cy="67" r="2.8" />
          <path className="cr-line" d="M40 61 L56 62" />
          <path className="cr-line" d="M64 62 L80 61" />
          <path className="cr-line" d="M52 81 Q56 79 60 81 Q64 83 68 81" />
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
