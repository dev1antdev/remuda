// The zoo's voices: a species' chirp for a question, a finish or a merge, synthesized in the browser (nothing is fetched,
// nothing passes through remuda), only while the person switched them on. The tune and the timbre are zooLife's.

import { useEffect } from "react";
import { TUNES, voiceOf, type Call } from "@/features/roster/zooLife";
import type { Species } from "@/features/roster/zooWear";

let context: AudioContext | null = null;

// wakeVoices makes or resumes the audio context; inside a click or a key, the gesture a browser waits for, it plays
export function wakeVoices() {
  try {
    context ??= new AudioContext();
    if (context.state === "suspended") void context.resume();
  } catch {
    context = null;
  }
}

// sleepVoices suspends the context, so the audio device is let go while the voices are off or the zoo is not on screen
export function sleepVoices() {
  if (context?.state === "running") void context.suspend();
}

// armVoices keeps the voices awake while they are on: at once when the page already had a click or a key (used), else
// at the next one on target, so a reload with the voices on is not silent; the function it returns puts them to sleep
export function armVoices(target: EventTarget, used: boolean): () => void {
  const wake = () => wakeVoices();
  if (used) wakeVoices();
  target.addEventListener("pointerdown", wake, { capture: true });
  target.addEventListener("keydown", wake, { capture: true });
  return () => {
    target.removeEventListener("pointerdown", wake, { capture: true });
    target.removeEventListener("keydown", wake, { capture: true });
    sleepVoices();
  };
}

// useVoices holds the voices awake while the zoo is on screen with them switched on, and asleep otherwise
export function useVoices(on: boolean) {
  useEffect(() => (on ? armVoices(window, navigator.userActivation?.hasBeenActive ?? false) : undefined), [on]);
}

const NOTE_S = 0.11;

// sing plays a call in a species' voice, quietly; nothing when the context could not be made or the page is hidden
export function sing(species: Species, call: Call) {
  if (!context || context.state !== "running" || document.visibilityState !== "visible") return;
  const { wave, base } = voiceOf(species);
  const start = context.currentTime + 0.02;
  TUNES[call].forEach((step, i) => {
    const at = start + i * NOTE_S;
    const osc = context!.createOscillator();
    const gain = context!.createGain();
    osc.type = wave;
    osc.frequency.value = base * 2 ** (step / 12);
    gain.gain.setValueAtTime(0.0001, at);
    gain.gain.exponentialRampToValueAtTime(wave === "sine" ? 0.09 : 0.04, at + 0.015);
    gain.gain.exponentialRampToValueAtTime(0.0001, at + NOTE_S);
    osc.connect(gain).connect(context!.destination);
    osc.start(at);
    osc.stop(at + NOTE_S + 0.02);
  });
}
