// The sidebar's marks after Orca: an agent's shape — the spinner, the "Needs you" pill (a "!" square where a heading
// already says it), the watchdog's "!", a filled dot, a hollow ring — an open folder's counts, a folded head's "2 need
// you · 1 working", a muted workspace's bell. Every shape reads without its colour; the words are in the tooltips.

import type { CSSProperties } from "react";
import { Icon } from "@/ui";
import type { Session } from "./api";
import { muteWords, mutedUntil } from "./mute";
import { markOf } from "./radar";
import { needWords, workingWords, type Shape, type Tally } from "./sidebarAgents";
import { t } from "@/lib/i18n";

// AgentShape is one agent's state mark; compact, a question is the "!" square
export function AgentShape({ shape, step, compact, style }: { shape: Shape; step: number; compact?: boolean; style?: CSSProperties }) {
  if (shape === "working")
    return (
      <span className="agent-shape is-working" style={style} aria-label={t("working")}>
        {markOf("working", step)}
      </span>
    );
  if (shape === "asking")
    return compact ? (
      <span className="agent-shape is-asking-square" aria-label={t("Needs you")}>
        !
      </span>
    ) : (
      <span className="needs-pill">{t("Needs you")}</span>
    );
  if (shape === "note")
    return (
      <span className="agent-shape is-note" aria-label={t("a watchdog note")}>
        !
      </span>
    );
  return <span className={`agent-shape is-${shape}`} aria-label={shape === "done" ? t("done") : t("idle")} />;
}

// MutedMark is the small bell-off a muted workspace wears, with how long the silence holds in its tooltip
export function MutedMark({ session }: { session: Session }) {
  const until = mutedUntil(session);
  if (!until) return null;
  return (
    <span className="radar-muted" title={muteWords(until)} aria-label={muteWords(until)}>
      <Icon name="bell-off" size={12} />
    </span>
  );
}

// StateMarks are an open folder's counts: how many work, ask, finished
export function StateMarks({ working, blocked, done, step }: { working: number; blocked: number; done: number; step: number }) {
  return (
    <>
      {working > 0 && (
        <span className="radar-mark working">
          {markOf("working", step)} {working}
        </span>
      )}
      {blocked > 0 && <span className="radar-mark blocked">? {blocked}</span>}
      {done > 0 && (
        <span className="radar-mark done">
          <Icon name="check" size={12} /> {done}
        </span>
      )}
    </>
  );
}

// HeadSummary is a folded folder's or machine's line after Orca: how many need you, as a chip, then how many work
// beside a spinner, then how many finished; quiet when nothing stirs
export function HeadSummary({ tally, step }: { tally: Tally; step: number }) {
  return (
    <span className="head-summary">
      {tally.needs > 0 && <span className="needs-chip">{needWords(tally.needs)}</span>}
      {tally.working > 0 && (
        <span className="head-working" title={workingWords(tally.working)}>
          <span className="agent-shape is-working">{markOf("working", step)}</span>
          <span className="head-working-words">{workingWords(tally.working)}</span>
          <span className="head-working-count">{tally.working}</span>
        </span>
      )}
      {tally.done > 0 && (
        <span className="radar-mark done" title={t("finished, not looked at yet")}>
          <Icon name="check" size={12} /> {tally.done}
        </span>
      )}
    </span>
  );
}
