// The audio context the page's alert sounds ring on: made or resumed inside a click or a key, the gesture a browser
// waits for before it lets a page make a sound — the play buttons in the alerts panel, or the page's next click after
// a reload with sounds on — and let go while they are off. The zoo's voices keep a context of their own.

import { useEffect } from "react";
import { due, play, soundFor, type Sound, type SoundChoice } from "./sounds";

let context: AudioContext | null = null;
let lastRang = -Infinity;

// wakeSounds makes or resumes the context; inside a gesture it may play from then on
export function wakeSounds() {
  try {
    context ??= new AudioContext();
    if (context.state === "suspended") void context.resume();
  } catch {
    context = null;
  }
}

// sleepSounds suspends the context, so the audio device is let go while the sounds are off
export function sleepSounds() {
  if (context?.state === "running") void context.suspend();
}

// armSounds keeps the context awake while sounds are on: at once when the page already had a click or a key (used),
// else at the next one on target; the function it returns puts it to sleep
export function armSounds(target: EventTarget, used: boolean): () => void {
  const wake = () => wakeSounds();
  if (used) wakeSounds();
  target.addEventListener("pointerdown", wake, { capture: true });
  target.addEventListener("keydown", wake, { capture: true });
  return () => {
    target.removeEventListener("pointerdown", wake, { capture: true });
    target.removeEventListener("keydown", wake, { capture: true });
    sleepSounds();
  };
}

// useSounds holds the context awake while the device's pick is a sound
export function useSounds(choice: SoundChoice) {
  const on = choice !== "off";
  useEffect(() => (on ? armSounds(window, navigator.userActivation?.hasBeenActive ?? false) : undefined), [on]);
}

// ring plays the one sound for alerts shown together, unless one rang within the gap or the context cannot play yet
export function ring(kinds: readonly string[], choice: SoundChoice, now = Date.now()): Sound | null {
  const sound = soundFor(kinds, choice);
  if (!sound || !due(now, lastRang) || context?.state !== "running") return null;
  lastRang = now;
  play(context, sound);
  return sound;
}

// preview plays a sound from a click on its button, whatever the pick, the gap or the quiet hours: the person asked;
// with the sounds off the device is let go again once it has rung
export function preview(sound: Sound, keepAwake: boolean) {
  wakeSounds();
  if (!context) return;
  const end = play(context, sound);
  if (!keepAwake) setTimeout(sleepSounds, Math.max(0, end - context.currentTime) * 1000 + 200);
}
