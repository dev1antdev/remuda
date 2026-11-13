// ActivityRibbon: what an agent did in the last half hour, one SVG under a tile's state line and under the chat's timeline: a
// stretch per call in the colour of its kind, the gaps between them as thought, now at the right. A hover or a tap names
// the stretch in the line under it, with the kind's icon, so the colour is never the only cue; ← → walk the stretches.
// A row of the View menu and the palette switch it (useRibbonShown, pref ribbon).

import { memo, useMemo, useRef, useState, type KeyboardEvent } from "react";
import { useNow } from "@/lib/clock";
import { usePref } from "@/lib/prefs";
import { Icon, type IconName } from "@/ui";
import { t } from "@/lib/i18n";
import type { Turn } from "./api";
import { ribbon, stepFrom, stretchAt, stretchWords, toneWords, turnWords, type Live, type Stretch, type Tone } from "./ribbon";
import { useChat } from "./useChat";

export const RIBBON_PREF = "ribbon";

// useRibbonShown is the per-device switch, on unless turned off
export const useRibbonShown = () => usePref<boolean>(RIBBON_PREF, true);

const TONE_ICONS: Record<Tone, IconName> = { think: "sparkles", read: "eye", write: "edit", shell: "terminal", wait: "clock", tool: "plug", quiet: "pause", unread: "eye-off" };
const UNITS = 1000; // the SVG's width in its own units, stretched to the ribbon's
const NARROW = 3; // a stretch narrower than this many units is drawn as a line two pixels wide

// saysItself: a call that reads, writes or runs a command is named by its tool, the kind's word beside it would repeat it
const saysItself = (s: Stretch) => !!s.call && (s.tone === "read" || s.tone === "write" || s.tone === "shell");

const liveOf = (status: string): Live => (status === "run" ? "run" : status === "input" ? "input" : null);

export const Ribbon = memo(function Ribbon({ turns, status, dir, cut, unread, onJump, className = "" }: {
  turns: Turn[];
  status: string; // the pane's: run, input, done, idle
  dir: string; // a file is named from here
  cut: boolean; // the turns read may start after the window does
  unread: string; // what the time before them is, in words
  onJump?: (turn: string) => void; // the lens: the stretch held opens its turn
  className?: string;
}) {
  const [on] = useRibbonShown();
  const now = useNow(5000);
  const live = liveOf(status);
  const model = useMemo(() => ribbon(turns, { now, live, cut }), [turns, now, live, cut]);
  const [hover, setHover] = useState<string | null>(null);
  const [held, setHeld] = useState<string | null>(null);
  const svg = useRef<SVGSVGElement>(null);
  const shown = model.stretches.find((s) => s.key === (held ?? hover)) ?? null;
  const span = model.now - model.from || 1;
  const x = (ms: number) => ((ms - model.from) / span) * UNITS;
  const under = (e: { clientX: number }): Stretch | null => {
    const box = svg.current?.getBoundingClientRect();
    return box && box.width > 0 ? stretchAt(model, (e.clientX - box.left) / box.width, box.width) : null;
  };
  const onKey = (e: KeyboardEvent) => {
    if (e.altKey || e.ctrlKey || e.metaKey) return;
    if (e.key === "ArrowRight" || e.key === "ArrowLeft") {
      e.preventDefault();
      setHeld(stepFrom(model, held, e.key === "ArrowRight" ? 1 : -1)?.key ?? null);
    } else if (e.key === "Escape" && held) {
      e.stopPropagation();
      setHeld(null);
    } else if (e.key === "Enter" && shown?.turn && onJump) onJump(shown.turn);
  };
  const base = model.stretches.filter((s) => !s.call);
  const calls = model.stretches.filter((s) => s.call);
  const draw = (s: Stretch) => {
    const from = x(s.from);
    const width = x(s.to) - from;
    const cls = `ribbon-s is-${s.tone}${s.key === shown?.key ? " is-shown" : ""}`;
    return width < NARROW ? (
      <line key={s.key} className={cls} data-tone={s.tone} x1={from + width / 2} x2={from + width / 2} y1={0} y2={10} vectorEffect="non-scaling-stroke" />
    ) : (
      <rect key={s.key} className={cls} data-tone={s.tone} x={from} width={width} y={0} height={10} />
    );
  };
  if (!on) return null;
  return (
    <div className={`ribbon ${className}`}>
      <div
        className="ribbon-track"
        tabIndex={0}
        role="group"
        aria-label={t("What the agent did in the last 30 minutes: {turn}. The arrow keys walk it.", { turn: turnWords(model.turn) })}
        onKeyDown={onKey}
        onBlur={() => setHover(null)}
      >
        <svg
          ref={svg}
          viewBox={`0 0 ${UNITS} 10`}
          preserveAspectRatio="none"
          onPointerMove={(e) => e.pointerType === "mouse" && setHover(under(e)?.key ?? null)}
          onPointerLeave={() => setHover(null)}
          onClick={(e) => {
            const s = under(e);
            setHeld(s && s.key !== held ? s.key : null);
          }}
        >
          <line className="ribbon-grid" x1={UNITS / 3} x2={UNITS / 3} y1={0} y2={10} vectorEffect="non-scaling-stroke" />
          <line className="ribbon-grid" x1={(UNITS * 2) / 3} x2={(UNITS * 2) / 3} y1={0} y2={10} vectorEffect="non-scaling-stroke" />
          {base.map(draw)}
          {calls.map(draw)}
          {calls
            .filter((s) => s.call?.failed)
            .map((s) => (
              <line key={`fail-${s.key}`} className="ribbon-fail" x1={x(s.to)} x2={x(s.to)} y1={0} y2={4} vectorEffect="non-scaling-stroke" />
            ))}
        </svg>
      </div>
      <div className="ribbon-line" aria-live="polite">
        {shown ? (
          <span className={`ribbon-named is-${shown.tone}${shown.call ? " is-call" : ""}`}>
            <Icon name={TONE_ICONS[shown.tone]} size={12} />
            {saysItself(shown) || <span className="ribbon-tone">{toneWords(shown.tone)}</span>}
            <span className="ribbon-what">{stretchWords(shown, model.now, dir, unread)}</span>
          </span>
        ) : (
          <span className="ribbon-turn">{turnWords(model.turn)}</span>
        )}
        {onJump && held && shown?.turn ? (
          <button type="button" className="linkish ribbon-jump" onClick={() => onJump(shown.turn)}>
            {t("Go to its turn")}
          </button>
        ) : (
          <span className="ribbon-scale">{t("30 min → now")}</span>
        )}
      </div>
    </div>
  );
});

// ScreenRibbon is the ribbon of a wall tile showing the pane's screen: it reads the same tail the chat view reads, at the
// same pace, only while the ribbon is on
export function ScreenRibbon({ machineId, paneId, status, dir, tailTurns }: { machineId: string; paneId: string; status: string; dir: string; tailTurns: number }) {
  const [shown] = useRibbonShown();
  const busy = status === "run" || status === "input";
  const { data } = useChat({ machineId, paneId, sessionId: null, dir, every: busy ? 3000 : 15000, active: shown, last: tailTurns });
  const turns = data?.turns ?? [];
  if (!shown) return null;
  return <Ribbon turns={turns} status={status} dir={dir} cut={turns.length >= tailTurns} unread={tailUnread(tailTurns)} className="tile-ribbon" />;
}

// tailUnread is a tile's word for the time before the turns it reads
export const tailUnread = (n: number) => t("Earlier than the {n} turns a tile reads; the chat lens has the whole conversation", { n });
