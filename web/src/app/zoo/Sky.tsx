// The zoo's surroundings: the sky of the viewer's hour behind it and in the strip over its pens (the sun by day, low and
// warm at dawn and dusk, the moon and stars by night), the marks of a pen's weather on its sign, and the fireworks of a merge over everything. Drawn
// faint behind the creatures, never under the pointer; the motion stops for reduced motion.

import type { Sky as SkyKind, Weather } from "@/features/roster/zooPens";
import { Icon } from "@/ui";

// STARS are where the stars stand in the sky's strip over the pens: across in percent, down in pixels
const STARS = [
  [4, 10],
  [11, 26],
  [19, 8],
  [27, 20],
  [35, 6],
  [43, 28],
  [51, 14],
  [59, 30],
  [66, 9],
  [74, 22],
  [82, 32],
  [96, 16],
] as const;

export function Sky({ sky }: { sky: SkyKind }) {
  return (
    <div className={`zoo-sky is-${sky}`} aria-hidden="true">
      {sky === "night" ? (
        <>
          <span className="zoo-moon" />
          {STARS.map(([left, top], i) => (
            <span key={i} className="zoo-star" style={{ left: `${left}%`, top: `${top}px`, animationDelay: `${(i % 4) * 0.7}s` }} />
          ))}
        </>
      ) : (
        <span className="zoo-sun" />
      )}
    </div>
  );
}

// WeatherMarks are a pen's weather as icons on its sign, beside the words that say it
export function WeatherMarks({ weather }: { weather: Weather }) {
  return (
    <span className="zoo-pen-marks" aria-hidden="true">
      {weather.heat && <Icon name="thermometer" size={12} className={`zoo-mark is-${weather.heat}`} />}
      {weather.storm && <Icon name="zap" size={12} className="zoo-mark is-storm" />}
      {weather.dust && <Icon name="chart" size={12} className="zoo-mark is-dust" />}
      {weather.boxes && <Icon name="archive" size={12} className="zoo-mark is-boxes" />}
      {weather.lamp && <Icon name="sun" size={12} className="zoo-mark is-lamp" />}
    </span>
  );
}

// Boxes are the cartons piled in the corner of a pen whose machine's disk is nearly full
export function Boxes() {
  return (
    <svg className="zoo-boxes" viewBox="0 0 60 34" aria-hidden="true">
      <rect x="2" y="14" width="24" height="18" rx="2" />
      <rect x="28" y="18" width="20" height="14" rx="2" />
      <rect x="11" y="1" width="20" height="14" rx="2" />
      <path d="M14 14 L14 32 M2 22 L26 22 M38 18 L38 32 M21 1 L21 15" />
    </svg>
  );
}

const BURSTS = [
  { left: 22, top: 26, delay: 0 },
  { left: 74, top: 18, delay: 0.6 },
  { left: 50, top: 40, delay: 1.2 },
  { left: 34, top: 12, delay: 1.9 },
  { left: 64, top: 44, delay: 2.5 },
];
const RAYS = Array.from({ length: 12 }, (_, i) => (i * Math.PI) / 6);

// Fireworks go off over the zoo for a merged pull request
export function Fireworks() {
  return (
    <div className="zoo-fireworks" aria-hidden="true">
      {BURSTS.map((burst, i) => (
        <svg key={i} className={`zoo-burst is-${i % 4}`} viewBox="-50 -50 100 100" style={{ left: `${burst.left}%`, top: `${burst.top}%`, animationDelay: `${burst.delay}s` }}>
          {RAYS.map((angle, j) => (
            <line key={j} x1={Math.cos(angle) * 12} y1={Math.sin(angle) * 12} x2={Math.cos(angle) * 44} y2={Math.sin(angle) * 44} />
          ))}
        </svg>
      ))}
    </div>
  );
}
