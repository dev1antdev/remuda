// PromptsList is the conversation prompt by prompt, newest first (prompts.ts): a column beside the log at a desk, a sheet
// over it on a phone. A card says when the prompt went, the messages sent while the agent worked, what the turn came to and
// the gist of the reply; opened in place it shows the prompt whole, those messages, the reply as Markdown and the files it
// changed, and the log jumps to its turn. ↑ ↓ walk the cards, Enter opens, Esc closes. After Over9k's Timeline.

import { useMemo, useRef, useState, type KeyboardEvent } from "react";
import { actions } from "@/app/store";
import type { OpenFile } from "@/features/files/target";
import type { PaneUsage } from "@/features/roster/api";
import { tokFine } from "@/features/usage/format";
import { t, tn } from "@/lib/i18n";
import { useNow } from "@/lib/clock";
import { usePref } from "@/lib/prefs";
import { whenShort } from "@/lib/time";
import { Icon, Tip } from "@/ui";
import { Markdown } from "@/ui/Markdown";
import type { Turn } from "./api";
import { shownPath } from "./footprint";
import { spinnerSeconds, type LiveRead } from "./progress";
import { alsoWords, facts, gist, plainText, promptCards, totalOutput, walk, type PromptCard, type Walk } from "./prompts";

export const PROMPTS_PREF = "chat.prompts";

// usePromptsShown is the per-device switch, off until the person opens the list
export const usePromptsShown = () => usePref<boolean>(PROMPTS_PREF, false);


export function PromptsList({ turns, live, usage, machineId, paneId, session, dir, onOpenFile, sheet }: {
  turns: Turn[];
  live: LiveRead | null; // the agent works: the newest card is the turn under way
  usage?: PaneUsage;
  machineId: string;
  paneId: string;
  session?: string;
  dir: string;
  onOpenFile: OpenFile;
  sheet: boolean; // a phone: over the log, from the bottom
}) {
  const [, setShown] = usePromptsShown();
  const oldest = useMemo(() => promptCards(turns), [turns]);
  const cards = useMemo(() => oldest.slice().reverse(), [oldest]);
  const [walked, setWalked] = useState<Walk>({ at: 0, open: null });
  const heads = useRef<(HTMLButtonElement | null)[]>([]);
  const at = Math.min(walked.at, Math.max(0, cards.length - 1));
  const jump = (card: PromptCard | undefined) => {
    if (card?.id && session) actions.jumpTo(machineId, paneId, session, card.id);
  };
  const move = (next: Walk) => {
    if (next.open !== null && next.open !== walked.open) jump(cards[next.open]);
    setWalked(next);
    heads.current[next.at]?.focus();
  };
  const onKey = (e: KeyboardEvent) => {
    const next = walk({ at, open: walked.open }, e.key, cards.length);
    if (!next) return;
    e.preventDefault();
    e.stopPropagation();
    move(next);
  };
  const liveTokens = live?.progress?.tokens;
  const output = totalOutput(oldest, live ? { seconds: null, tokens: liveTokens } : null);
  const close = () => setShown(false);
  return (
    <>
      {sheet && <div className="prompts-scrim" aria-hidden onClick={close} />}
      <aside className={`prompts${sheet ? " is-sheet" : ""}`} aria-label={t("Prompts")}>
        <div className="prompts-head">
          <span className="prompts-title">{t("Prompts")}</span>
          {cards.length > 0 && <span className="badge quiet">{tn(cards.length, "{n} prompt", "{n} prompts")}</span>}
          <span className="prompts-meta">{[output ? `↓ ${tokFine(output)}` : "", usage?.context ? t("context {n}", { n: tokFine(usage.context) }) : ""].filter(Boolean).join(" · ")}</span>
          <Tip text={t("Close the prompts list")}>
            <button type="button" className="icon-btn sm" aria-label={t("Close the prompts list")} onClick={close}>
              <Icon name="x" size={14} />
            </button>
          </Tip>
        </div>
        {cards.length === 0 ? (
          <div className="empty prompts-empty">{t("No prompts yet.")}</div>
        ) : (
          <ol className="prompts-list" onKeyDown={onKey}>
            {cards.map((card, i) => (
              <Card
                key={card.id || card.n}
                card={card}
                live={i === 0 ? live : null}
                picked={i === at}
                open={walked.open === i}
                first={i === 0}
                last={i === cards.length - 1}
                dir={dir}
                onOpenFile={onOpenFile}
                headRef={(el) => (heads.current[i] = el)}
                onPick={() => {
                  const opening = walked.open !== i;
                  if (opening) jump(card);
                  setWalked({ at: i, open: opening ? i : null });
                }}
              />
            ))}
          </ol>
        )}
      </aside>
    </>
  );
}

function Card({ card, live, picked, open, first, last, dir, onOpenFile, headRef, onPick }: {
  card: PromptCard;
  live: LiveRead | null;
  picked: boolean;
  open: boolean;
  first: boolean;
  last: boolean;
  dir: string;
  onOpenFile: OpenFile;
  headRef: (el: HTMLButtonElement | null) => void;
  onPick: () => void;
}) {
  const also = alsoWords(card.also);
  const reply = gist(card.reply);
  return (
    <li className={`prompt${live ? " is-live" : ""}${picked ? " is-picked" : ""}${open ? " is-open" : ""}${first ? " is-first" : ""}${last ? " is-last" : ""}`}>
      <span className="prompt-rail" aria-hidden>
        <span className="prompt-dot" />
      </span>
      <div className="prompt-card">
        <button type="button" className="linkish prompt-head" ref={headRef} aria-expanded={open} tabIndex={picked ? 0 : -1} onClick={onPick}>
          <span className="prompt-when">
            #{card.n} {card.at ? whenShort(card.at) : ""}
          </span>
          {!open && <span className="prompt-text">{plainText(card.text)}</span>}
          {!open && also && <span className="prompt-also">{also}</span>}
          {live ? <LiveFacts card={card} live={live} /> : <Facts card={card} />}
          {!open && reply && <span className="prompt-gist">{reply}</span>}
        </button>
        {open && <Opened card={card} live={!!live} dir={dir} onOpenFile={onOpenFile} />}
      </div>
    </li>
  );
}

function Facts({ card }: { card: PromptCard }) {
  const said = facts(card, null, (ms) => whenShort(ms));
  return said.rest.length ? <span className="prompt-facts">{said.rest.join(" · ")}</span> : null;
}

// LiveFacts is the turn under way: "working" in the working colour, its time ticking on from the spinner's
function LiveFacts({ card, live }: { card: PromptCard; live: LiveRead }) {
  const now = useNow(1000);
  const said = facts(card, { seconds: spinnerSeconds(live.progress, live.readAt, now, live.since || undefined), tokens: live.progress?.tokens }, (ms) => whenShort(ms));
  return (
    <span className="prompt-facts">
      <span className="prompt-working">{said.lead}</span>
      {said.rest.length > 0 && ` · ${said.rest.join(" · ")}`}
    </span>
  );
}

// Opened is a card read in full: the prompt, the messages sent while the agent worked with their times, the reply as Markdown, the files it changed
function Opened({ card, live, dir, onOpenFile }: { card: PromptCard; live: boolean; dir: string; onOpenFile: OpenFile }) {
  return (
    <div className="prompt-body">
      <div className="prompt-full part-text">{card.text}</div>
      {card.also.length > 0 && (
        <ul className="prompt-sent">
          {card.also.map((m, i) => (
            <li key={i}>
              <span className="prompt-sent-at">{m.at ? whenShort(m.at) : ""}</span>
              <span className="prompt-sent-text">
                {m.from && <b>{m.from}: </b>}
                {m.text}
              </span>
            </li>
          ))}
        </ul>
      )}
      <div className="prompt-reply">{card.reply ? <Markdown text={card.reply} onOpenFile={onOpenFile} /> : <span className="dim">{live ? t("Working…") : t("No reply text.")}</span>}</div>
      {card.edits.length > 0 && (
        <div className="prompt-files">
          <span className="prompt-files-head">{t("Changed")}</span>
          {card.edits.map((path) => (
            <button key={path} type="button" className="linkish path-link" title={`${path}\n${t("Open the file with its changes")}`} onClick={() => onOpenFile({ path, prefer: "changes" })}>
              <bdi dir="ltr">{shownPath(path, dir)}</bdi>
            </button>
          ))}
        </div>
      )}
    </div>
  );
}

