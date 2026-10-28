// The day strip under an agent's heading: when it worked, waited and sat idle over the period, the hours under the bar,
// a click on a stretch of work opening the chat there. A phone shows the last eight hours.

import { useMemo } from "react";
import { useMedia } from "@/lib/media";
import type { Span } from "./api";
import { hourLabel, hourMarks, layout, spanWords, WIDTH, window, type Segment } from "./strip";

export function DayStrip({ spans, since, onJump }: { spans: Span[]; since?: string; onJump?: (turn: string) => void }) {
  const phone = useMedia("(max-width: 600px)");
  const { segments, marks } = useMemo(() => {
    const { from, to } = window(since, Date.now(), phone ? 8 : undefined);
    return { segments: layout(spans, from, to), marks: hourMarks(from, to, phone ? 6 : 3) };
  }, [spans, since, phone]);
  const linked = (seg: Segment) => Boolean(onJump && seg.turn && seg.state !== "idle");
  return (
    <div className="strip">
      <svg className="strip-bar" viewBox={`0 0 ${WIDTH} 8`} preserveAspectRatio="none" aria-label="When the agent worked, waited and sat idle">
        {segments.map((seg, i) => (
          <rect key={i} x={seg.x} y={0} width={seg.w} height={8} className={`strip-span is-${seg.state}${linked(seg) ? " is-link" : ""}`} onClick={() => linked(seg) && onJump!(seg.turn!)}>
            <title>{spanWords(seg)}</title>
          </rect>
        ))}
        {marks.map((m) => (
          <line key={m.at} x1={m.x} x2={m.x} y1={0} y2={8} className="strip-mark" vectorEffect="non-scaling-stroke" />
        ))}
      </svg>
      <div className="strip-hours">
        {marks
          .filter((m) => m.labelled)
          .map((m) => (
            <span key={m.at} style={{ left: `${(m.x / WIDTH) * 100}%` }}>
              {hourLabel(m.at)}
            </span>
          ))}
      </div>
    </div>
  );
}
