// The page's sounds for an alert shown in its corner, made in the browser with WebAudio — no file is fetched, nothing
// passes through remuda. After Orchi's: a two-tone bell when an agent needs you, a short rising run when one finishes,
// and the soft chime of before for anything else, or for every alert when that is the choice. At most one sound in
// SOUND_GAP_MS, so a burst of alerts after a reconnect rings once, with the most pressing of them.

// SoundChoice is the device's pick in the alerts panel: none, the chime for every alert, or the bell and the run
export type SoundChoice = "off" | "chime" | "bell";
export type Sound = "chime" | "bell" | "run";

// SOUNDS_KEY is the per-device pick; LEGACY_SOUND_KEY the switch it replaces, a chime when on
export const SOUNDS_KEY = "alerts.sounds";
export const LEGACY_SOUND_KEY = "alerts.sound";

// SOUND_GAP_MS is the least time between two sounds, as Orchi's workers finishing together play once
export const SOUND_GAP_MS = 1500;

// the alerts that ask for the person: a question, or the watchdog's word that one is stuck, waiting or looping
const NEEDS_YOU = new Set(["input", "stuck", "waiting", "loop"]);

const CHOICES: readonly SoundChoice[] = ["off", "chime", "bell"];

// choiceOf reads the stored pick, else the switch of before: on was the chime
export function choiceOf(stored: unknown, legacy: unknown): SoundChoice {
  if (CHOICES.includes(stored as SoundChoice)) return stored as SoundChoice;
  return legacy === true ? "chime" : "off";
}

// soundFor is the one sound a set of alerts shown together rings with: the bell over the run over the chime
export function soundFor(kinds: readonly string[], choice: SoundChoice): Sound | null {
  if (choice === "off" || kinds.length === 0) return null;
  if (choice === "chime") return "chime";
  if (kinds.some((kind) => NEEDS_YOU.has(kind))) return "bell";
  if (kinds.includes("done")) return "run";
  return "chime";
}

// due says whether a sound may ring now, the last one having rung at `last` (ms)
export const due = (now: number, last: number): boolean => now - last >= SOUND_GAP_MS;

// Note is one tone of a sound: its pitch, when it starts after the sound does and how long it rings (s), how loud at
// its peak, the oscillator's wave, and the partials over the fundamental — ratio and weight — that make a bell of a sine
export type Note = { frequency: number; at: number; length: number; peak: number; wave: OscillatorType; partials: readonly (readonly [number, number])[] };

// a struck bell: mostly the note, a little of the octave and the twelfth, a trace of the bell's sharp fourth partial
const BELL: readonly (readonly [number, number])[] = [
  [1, 1],
  [2, 0.32],
  [3, 0.12],
  [4.2, 0.06],
];
const PURE: readonly (readonly [number, number])[] = [[1, 1]];

// notesOf is a sound's score: the chime two soft notes up a fourth, the bell a doorbell's ding and dong a major third
// apart, the run a chiptune's C major arpeggio, its top note held
export function notesOf(sound: Sound): Note[] {
  switch (sound) {
    case "chime":
      return [
        { frequency: 660, at: 0, length: 0.18, peak: 0.08, wave: "sine", partials: PURE },
        { frequency: 880, at: 0.12, length: 0.18, peak: 0.08, wave: "sine", partials: PURE },
      ];
    case "bell":
      return [
        { frequency: 659.25, at: 0, length: 0.9, peak: 0.1, wave: "sine", partials: BELL },
        { frequency: 523.25, at: 0.34, length: 1.1, peak: 0.1, wave: "sine", partials: BELL },
      ];
    case "run":
      return [523.25, 659.25, 783.99, 1046.5].map((frequency, i) => ({ frequency, at: i * 0.075, length: i === 3 ? 0.3 : 0.09, peak: 0.035, wave: "square", partials: PURE }));
  }
}

// AudioLike is what a sound is drawn on: an AudioContext, or a stand-in that keeps what was asked of it
export type AudioLike = Pick<BaseAudioContext, "currentTime" | "destination" | "createOscillator" | "createGain">;

// play rings a sound on a context now: each partial of each note an oscillator through its own gain, struck and
// decaying; the moment the last one ends is answered
export function play(context: AudioLike, sound: Sound): number {
  const start = context.currentTime + 0.02;
  let end = start;
  for (const note of notesOf(sound)) {
    for (const [ratio, weight] of note.partials) {
      const at = start + note.at;
      const tone = context.createOscillator();
      const volume = context.createGain();
      tone.type = note.wave;
      tone.frequency.value = note.frequency * ratio;
      volume.gain.setValueAtTime(0.0001, at);
      volume.gain.exponentialRampToValueAtTime(note.peak * weight, at + 0.008);
      volume.gain.exponentialRampToValueAtTime(0.0001, at + note.length / ratio ** 0.5); // a higher partial dies sooner
      tone.connect(volume).connect(context.destination);
      tone.start(at);
      tone.stop(at + note.length + 0.02);
      end = Math.max(end, at + note.length + 0.02);
    }
  }
  return end;
}
