// The zoo's voices: a species' chirp for a question, a finish or a merge, synthesized in the browser (nothing is fetched,
// nothing passes through remuda), only while the person switched them on. The tune and the timbre are zooLife's.

import { TUNES, voiceOf, type Call } from "@/features/roster/zooLife";
import type { Species } from "@/features/roster/zooWear";

let context: AudioContext | null = null;

// wakeVoices makes the audio context inside a click, the gesture a browser waits for before it plays anything
export function wakeVoices() {
  try {
    context ??= new AudioContext();
    void context.resume();
  } catch {
    context = null;
  }
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
