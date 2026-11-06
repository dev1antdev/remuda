// One small chart of a day's samples: the line, a light fill under it, and the thresholds as dashed lines in their
// tone; pure SVG, stretched to its row's width.

import { areaPath, sparkPath, thresholdY, type Threshold } from "./spark";

const WIDTH = 160;
const HEIGHT = 28;

export function Sparkline({ values, min, max, color, thresholds, title }: { values: number[]; min: number; max: number; color: string; thresholds: Threshold[]; title: string }) {
  const line = sparkPath(values, WIDTH, HEIGHT, min, max);
  return (
    <svg className="spark" viewBox={`0 0 ${WIDTH} ${HEIGHT}`} preserveAspectRatio="none" role="img" aria-label={title}>
      <title>{title}</title>
      <path className="spark-area" d={areaPath(line, WIDTH, HEIGHT)} style={{ fill: color }} />
      {thresholds.map((t) => {
        const y = thresholdY(t.value, min, max, HEIGHT);
        return y === null ? null : <line key={t.tone} className={`spark-${t.tone}`} x1={0} x2={WIDTH} y1={y} y2={y} />;
      })}
      <path className="spark-line" d={line} style={{ stroke: color }} />
    </svg>
  );
}
