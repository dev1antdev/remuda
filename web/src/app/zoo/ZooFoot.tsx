// The zoo's foot: a line telling the day — answers, waits, spend, merges, who worked longest, from the standup since
// midnight read every DAY_EVERY while the zoo is in view and on coming back into view — running along when it is longer
// than the screen is wide, and the switch of the creatures' voices beside it.

import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { dayDue } from "@/features/roster/zooLife";
import { fetchStandup, type Standup } from "@/features/standup/api";
import { t } from "@/lib/i18n";
import { pageVisible, watchVisibility } from "@/lib/visibility";
import { Icon, Tip } from "@/ui";

// LOOK_EVERY is how often the foot looks whether the day is due a reading
const LOOK_EVERY = 60_000;

// useDay is the standup since this device's midnight, kept fresh while the page is in view
export function useDay(): Standup | null {
  const [day, setDay] = useState<Standup | null>(null);
  useEffect(() => {
    let stopped = false;
    let readAt = 0;
    const tz = Intl.DateTimeFormat().resolvedOptions().timeZone;
    const read = () => {
      if (!pageVisible() || !dayDue(readAt, Date.now())) return;
      readAt = Date.now();
      fetchStandup(undefined, tz).then(
        (standup) => !stopped && setDay(standup),
        () => {
          readAt = 0;
        },
      );
    };
    read();
    const id = setInterval(read, LOOK_EVERY);
    const unwatch = watchVisibility(read);
    return () => {
      stopped = true;
      clearInterval(id);
      unwatch();
    };
  }, []);
  return day;
}

export function ZooFoot({ facts, voices, onVoices }: { facts: string[]; voices: boolean; onVoices: () => void }) {
  const text = facts.length ? facts.join("  ·  ") : t("A quiet day so far");
  const box = useRef<HTMLDivElement>(null);
  const copy = useRef<HTMLSpanElement>(null);
  const [moving, setMoving] = useState(false);
  useLayoutEffect(() => {
    const measure = () => setMoving(!!copy.current && !!box.current && copy.current.scrollWidth > box.current.clientWidth + 1);
    measure();
    const observer = typeof ResizeObserver === "function" ? new ResizeObserver(measure) : null;
    if (box.current) observer?.observe(box.current);
    return () => observer?.disconnect();
  }, [text]);
  return (
    <footer className="zoo-foot">
      <div className={`zoo-ticker${moving ? " is-moving" : ""}${facts.length ? "" : " is-quiet"}`} ref={box}>
        <div className="zoo-ticker-track" style={{ animationDuration: `${Math.max(18, Math.round(text.length / 4))}s` }}>
          <span className="zoo-ticker-copy" ref={copy}>
            {text}
          </span>
          {moving && (
            <span className="zoo-ticker-copy" aria-hidden="true">
              {text}
            </span>
          )}
        </div>
      </div>
      <Tip text={voices ? t("Voices on: a creature chirps when it asks, finishes or has its pull request merged") : t("Voices off: switch on to hear a creature ask, finish or get merged")}>
        <button type="button" className={`icon-btn sm${voices ? " on" : ""}`} aria-label={t("The zoo's voices")} aria-pressed={voices} onClick={onVoices}>
          <Icon name="volume" size={14} />
        </button>
      </Tip>
    </footer>
  );
}
