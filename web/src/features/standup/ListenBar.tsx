// Reading aloud in a panel: the Listen button in its head, and while it reads, a bar over the report — play or pause,
// back, skip, the sentence being said under the part it belongs to, stop — with what an iPhone will not do said there.

import { t } from "@/lib/i18n";
import { isAppleTouch, markPrimed, nowSaid, speechSupported, type Part } from "@/lib/speech";
import { Icon, Notice, Tip } from "@/ui";
import type { Listening } from "./useListen";

// errorWords is why the browser did not speak, in words
function errorWords(error: string | undefined): string {
  if (error === "not-allowed") return t("The browser wants a tap before it speaks: press Listen again.");
  if (error === "no-voice") return t("This browser has no voice to speak with.");
  return t("The browser could not speak ({error}).", { error: error ?? "" });
}

const appleTouch = typeof navigator !== "undefined" && isAppleTouch(navigator.userAgent, navigator.maxTouchPoints ?? 0);

export function ListenButton({ listening, script, ready, what }: { listening: Listening; script: () => Part[]; ready: boolean; what: string }) {
  const can = speechSupported();
  const reading = listening.state !== null && listening.state.status !== "idle";
  // the first lines are handed to the voice inside this click, which is what Safari on iOS waits for
  const begin = () => {
    listening.start(script());
    markPrimed();
  };
  return (
    <Tip text={!can ? t("This browser has no voice to speak with.") : reading ? t("Stop reading") : what}>
      <button type="button" className={`btn sm listen-start${reading ? " on" : ""}`} disabled={!can || (!ready && !reading)} aria-pressed={reading} onClick={reading ? listening.stop : begin}>
        <Icon name="headphones" size={14} /> {t("Listen")}
      </button>
    </Tip>
  );
}

export function ListenBar({ listening, back, skip }: { listening: Listening; back: string; skip: string }) {
  const { state, parts } = listening;
  if (!state || state.status === "idle") return null;
  const { title, text } = nowSaid(parts, state);
  const playing = state.status === "playing";
  const replay = state.status === "done" || state.status === "failed";
  const onToggle = () => {
    listening.toggle();
    markPrimed();
  };
  return (
    <section className={`listen-bar is-${state.status}`} aria-label={t("Reading aloud")}>
      <div className="listen-controls">
        <Tip text={playing ? t("Pause") : replay ? t("From the start") : t("Play")}>
          <button type="button" className="icon-btn listen-toggle" aria-label={playing ? t("Pause") : replay ? t("From the start") : t("Play")} onClick={onToggle}>
            <Icon name={playing ? "pause" : "play"} size={16} />
          </button>
        </Tip>
        <Tip text={back}>
          <button type="button" className="icon-btn listen-back" aria-label={back} disabled={replay} onClick={listening.previous}>
            <Icon name="skip-back" size={16} />
          </button>
        </Tip>
        <Tip text={skip}>
          <button type="button" className="icon-btn listen-skip" aria-label={skip} disabled={replay} onClick={listening.next}>
            <Icon name="skip-forward" size={16} />
          </button>
        </Tip>
      </div>
      <div className="listen-now" aria-live="polite">
        <span className="listen-where">
          {title}
          {parts.length > 1 && <span className="listen-count"> · {t("{n} of {total}", { n: state.part + 1, total: parts.length })}</span>}
        </span>
        <span className="listen-sentence">{state.status === "done" ? t("That was all.") : text}</span>
      </div>
      <Tip text={t("Stop reading")}>
        <button type="button" className="icon-btn listen-stop" aria-label={t("Stop reading")} onClick={listening.stop}>
          <Icon name="x" size={16} />
        </button>
      </Tip>
      {state.status === "failed" && (
        <Notice tone="warn" className="listen-note">
          {errorWords(state.error)}
        </Notice>
      )}
      {appleTouch && state.status !== "failed" && <div className="listen-note quiet">{t("On an iPhone or iPad, Safari stops speaking when the screen locks or another app comes to the front: keep the screen on while you listen.")}</div>}
    </section>
  );
}
