// TimelineBar is the thin bar over a conversation: its prompts and answers laid out in time, the moments worth a
// look marked, every one a jump to its turn; a row of the View menu switches it (useTimelineShown, pref chat.timeline).

import { useMemo } from "react";
import { actions } from "@/app/store";
import { isTouch, usePref } from "@/lib/prefs";
import { spent, whenShort } from "@/lib/time";
import { Icon } from "@/ui";
import type { Turn } from "./api";
import { chips, summary, timeline } from "./timeline";

export const TIMELINE_PREF = "chat.timeline";

// useTimelineShown is the per-device switch: on with a mouse, off on touch, where the bar would cost a thumb's width
export const useTimelineShown = () => usePref<boolean>(TIMELINE_PREF, !isTouch);

const pct = (x: number) => `${(100 * x).toFixed(3)}%`;

export function TimelineBar({ turns, machineId, paneId, session }: { turns: Turn[]; machineId: string; paneId: string; session?: string }) {
  const [shown] = useTimelineShown();
  const line = useMemo(() => (shown && turns.length >= 2 ? timeline(turns) : null), [shown, turns]);
  if (!line || !session || line.segments.length === 0) return null;
  const go = (id: string) => {
    if (id) actions.jumpTo(machineId, paneId, session, id);
  };
  const row = chips(line);
  return (
    <div className="timeline" title={summary(line)}>
      <div className="timeline-bar">
        {line.segments.map((s, i) => (
          <button
            key={s.id || i}
            type="button"
            className="linkish timeline-seg"
            style={{ left: pct(s.x), width: pct(s.w) }}
            title={`${whenShort(s.start)} · ${s.prompt}${s.took ? ` · ${spent(s.took)}` : ""}`}
            aria-label={s.prompt || "a prompt"}
            onClick={() => go(s.id)}
          />
        ))}
        {line.highlights.map((h, i) => (
          <button key={`${h.kind}${i}`} type="button" className={`linkish timeline-mark is-${h.kind}`} style={{ left: pct(h.x) }} title={`${h.label}: ${h.detail}`} aria-label={h.label} onClick={() => go(h.id)} />
        ))}
      </div>
      {row.length > 0 && (
        <div className="timeline-chips">
          {row.map((c, i) => (
            <button key={`${c.kind}${i}`} type="button" className={`linkish timeline-chip is-${c.kind}`} title={c.title} onClick={() => go(c.id)}>
              {c.text}
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

