import { afterAll, describe, expect, it } from "vitest";
import { choiceOf, due, notesOf, play, soundFor, SOUND_GAP_MS, type AudioLike } from "./sounds";
import { armSounds, ring } from "./player";

// Recorder stands in for an AudioContext: it keeps every oscillator asked of it, its pitch, wave and start, and the
// gain envelope it went through
type Tone = { type: string; frequency: number; start: number; stop: number; peak: number };
class Recorder {
  currentTime = 10;
  destination = {} as AudioDestinationNode;
  state: "suspended" | "running" = "suspended";
  tones: Tone[] = [];
  createOscillator() {
    const tone: Tone = { type: "", frequency: 0, start: 0, stop: 0, peak: 0 };
    this.tones.push(tone);
    const node = {
      set type(value: string) {
        tone.type = value;
      },
      frequency: {
        set value(hz: number) {
          tone.frequency = hz;
        },
      },
      connect: (gain: { tone?: Tone }) => {
        gain.tone = tone;
        return { connect: () => {} };
      },
      start: (at: number) => (tone.start = at),
      stop: (at: number) => (tone.stop = at),
    };
    return node as unknown as OscillatorNode;
  }
  createGain() {
    const gain: { tone?: Tone; gain: unknown } = {
      gain: {
        setValueAtTime: () => {},
        exponentialRampToValueAtTime: (value: number) => {
          if (gain.tone) gain.tone.peak = Math.max(gain.tone.peak, value);
        },
      },
    };
    return gain as unknown as GainNode;
  }
  resume() {
    this.state = "running";
    return Promise.resolve();
  }
  suspend() {
    this.state = "suspended";
    return Promise.resolve();
  }
}

const made: Recorder[] = [];
const browsers = globalThis.AudioContext;
globalThis.AudioContext = class extends Recorder {
  constructor() {
    super();
    made.push(this);
  }
} as unknown as typeof AudioContext;
afterAll(() => {
  globalThis.AudioContext = browsers;
});

describe("the sound an alert rings with", () => {
  it("is none when off, the chime for every alert, or the bell over the run over the chime", () => {
    expect(soundFor(["input"], "off")).toBeNull();
    expect(soundFor([], "bell")).toBeNull();
    expect(soundFor(["done", "input"], "chime")).toBe("chime");
    expect(soundFor(["done", "stuck"], "bell")).toBe("bell");
    expect(soundFor(["review", "done"], "bell")).toBe("run");
    expect(soundFor(["digest"], "bell")).toBe("chime");
  });
  it("reads the switch of before as the chime, and an unknown pick as off", () => {
    expect(choiceOf("bell", true)).toBe("bell");
    expect(choiceOf(undefined, true)).toBe("chime");
    expect(choiceOf("loud", false)).toBe("off");
  });
  it("rings at most once in the gap", () => {
    expect(due(1000, -Infinity)).toBe(true);
    expect(due(1000 + SOUND_GAP_MS - 1, 1000)).toBe(false);
    expect(due(1000 + SOUND_GAP_MS, 1000)).toBe(true);
  });
});

describe("a sound's score", () => {
  it("is a doorbell for the bell: a ding and a lower dong, each with its partials", () => {
    const [ding, dong] = notesOf("bell");
    expect(dong!.frequency).toBeLessThan(ding!.frequency);
    expect(dong!.at).toBeGreaterThan(ding!.at);
    expect(ding!.partials.length).toBeGreaterThan(1);
  });
  it("is a rising run for the finish, short, its top note held", () => {
    const run = notesOf("run");
    expect(run.map((note) => note.frequency)).toEqual([...run.map((note) => note.frequency)].sort((a, b) => a - b));
    expect(run.at(-1)!.length).toBeGreaterThan(run[0]!.length);
    expect(run.at(-1)!.at + run.at(-1)!.length).toBeLessThan(0.7);
  });
  it("plays every partial of every note from now, quietly", () => {
    const context = new Recorder();
    const end = play(context as unknown as AudioLike, "bell");
    expect(context.tones).toHaveLength(notesOf("bell").reduce((n, note) => n + note.partials.length, 0));
    expect(Math.min(...context.tones.map((tone) => tone.start))).toBeGreaterThanOrEqual(10);
    expect(Math.max(...context.tones.map((tone) => tone.peak))).toBeLessThanOrEqual(0.12);
    expect(end).toBe(Math.max(...context.tones.map((tone) => tone.stop)));
    expect(context.tones.filter((tone) => tone.frequency === 659.25)).toHaveLength(1);
  });
});

describe("the page's sounds", () => {
  it("wait for the first click after a reload, then ring at most once in the gap, and let the device go when off", () => {
    const page = new EventTarget();
    const disarm = armSounds(page, false);
    expect(ring(["input"], "bell", 100_000)).toBeNull(); // no gesture yet: nothing can play
    expect(made).toHaveLength(0);
    page.dispatchEvent(new Event("pointerdown"));
    const context = made[0]!;
    expect(context.state).toBe("running");
    expect(ring(["input"], "bell", 100_000)).toBe("bell");
    expect(ring(["done"], "bell", 100_500)).toBeNull();
    expect(ring(["done"], "bell", 100_000 + SOUND_GAP_MS)).toBe("run");
    expect(context.tones.some((tone) => tone.type === "square")).toBe(true);
    disarm();
    expect(context.state).toBe("suspended");
    expect(ring(["input"], "bell", 200_000)).toBeNull();
  });
});
