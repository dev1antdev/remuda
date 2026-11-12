import { afterEach, describe, expect, it } from "vitest";
import { createPlayer, isAppleTouch, nowSaid, pickVoice, primeSpeech, resetPriming, speakScript, whenPrimed, type Env, type Part, type PlayState, type SynthLike, type UtteranceLike, type VoiceLike } from "./speech";

const VOICES: VoiceLike[] = [
  { name: "Samantha", lang: "en-US", default: true, localService: true },
  { name: "Daniel", lang: "en-GB", localService: true },
  { name: "Google русский", lang: "ru-RU" },
  { name: "Milena", lang: "ru-RU", localService: true },
];

// stand is a stand-in voice: it keeps what it was handed and says each line when the test asks it to
function stand(voices = VOICES) {
  const queue: UtteranceLike[] = [];
  const spoken: UtteranceLike[] = [];
  const later: (() => void)[] = [];
  let cancels = 0;
  const synth: SynthLike = {
    speak: (line) => queue.push(line),
    cancel: () => {
      cancels++;
      const dropped = queue.splice(0);
      for (const line of dropped) line.onerror?.({ error: "interrupted" });
    },
    getVoices: () => voices,
    get speaking() {
      return queue.length > 0;
    },
    get pending() {
      return queue.length > 1;
    },
  };
  const env: Env = {
    synth: () => synth,
    utter: (text) => ({ text, lang: "", voice: null, pitch: 1, rate: 1, volume: 1, onstart: null, onend: null, onerror: null }),
    later: (fn) => later.push(fn),
  };
  // flush runs what followed a cancel, as the browser's beat after it would
  const flush = () => later.splice(0).forEach((fn) => fn());
  // begin starts the line at the head of the queue, end finishes it
  const begin = () => queue[0]?.onstart?.();
  const end = () => {
    const line = queue.shift();
    if (line) spoken.push(line);
    line?.onend?.();
    queue[0]?.onstart?.();
  };
  return { env, queue, spoken, begin, end, flush, cancels: () => cancels };
}

const PARTS: Part[] = [
  { key: "intro", title: "Standup", sentences: [{ text: "Good morning.", lang: "en" }, { text: "Here is your day so far.", lang: "en" }] },
  { key: "local|p1", title: "api · Claude Code", sentences: [{ text: "In api, Claude Code has finished.", lang: "en" }, { text: "Все тесты прошли.", lang: "ru" }] },
  { key: "closing", title: "What is left", sentences: [{ text: "That's all for now.", lang: "en" }] },
];

const watch = () => {
  const states: PlayState[] = [];
  return { states, onChange: (state: PlayState) => states.push(state), last: () => states[states.length - 1] };
};

afterEach(() => resetPriming());

describe("pickVoice", () => {
  it("keeps the voice picked for its own language and for a sentence that names none", () => {
    expect(pickVoice(VOICES, "en", "Daniel")?.name).toBe("Daniel");
    expect(pickVoice(VOICES, "", "Milena")?.name).toBe("Milena");
  });
  it("speaks another language in a voice of that language: the browser's default for it, then one on the device", () => {
    expect(pickVoice(VOICES, "ru", "Daniel")?.name).toBe("Milena");
    expect(pickVoice(VOICES, "en")?.name).toBe("Samantha");
    expect(pickVoice([{ name: "Remote", lang: "ru_RU" }], "ru")?.name).toBe("Remote");
  });
  it("falls back to the voice picked, else leaves the choice to the browser", () => {
    expect(pickVoice(VOICES, "de", "Daniel")?.name).toBe("Daniel");
    expect(pickVoice(VOICES, "de")).toBeUndefined();
    expect(pickVoice([], "en")).toBeUndefined();
  });
});

describe("the player", () => {
  it("queues every sentence at once in its language's voice and follows the one being said", () => {
    const voice = stand();
    const seen = watch();
    const player = createPlayer(PARTS, { voiceName: "Daniel", onChange: seen.onChange }, voice.env);
    player.play();
    expect(voice.queue.map((line) => line.text)).toEqual(["Good morning.", "Here is your day so far.", "In api, Claude Code has finished.", "Все тесты прошли.", "That's all for now."]);
    expect(voice.queue.map((line) => line.voice?.name)).toEqual(["Daniel", "Daniel", "Daniel", "Milena", "Daniel"]);
    expect(seen.last()).toEqual({ status: "playing", part: 0, sentence: 0 });
    voice.begin();
    voice.end();
    voice.end();
    expect(seen.last()).toEqual({ status: "playing", part: 1, sentence: 0 });
    expect(nowSaid(PARTS, seen.last()!)).toEqual({ title: "api · Claude Code", text: "In api, Claude Code has finished." });
    voice.end();
    voice.end();
    voice.end();
    expect(seen.last()).toEqual({ status: "done", part: 2, sentence: 0 });
  });

  it("pauses where it is and goes on from the sentence it stopped in", () => {
    const voice = stand();
    const seen = watch();
    const player = createPlayer(PARTS, { onChange: seen.onChange }, voice.env);
    player.play();
    voice.begin();
    voice.end();
    player.pause();
    expect(seen.last()).toEqual({ status: "paused", part: 0, sentence: 1 });
    expect(voice.queue).toEqual([]);
    expect(seen.states.filter((state) => state.status === "failed")).toEqual([]); // the cancel's own errors are ours
    player.toggle();
    expect(voice.queue[0]?.text).toBe("Here is your day so far.");
    expect(seen.last()?.status).toBe("playing");
  });

  it("skips to the next part and back to the start of this one, or the one before from its start", () => {
    const voice = stand();
    const seen = watch();
    const player = createPlayer(PARTS, { onChange: seen.onChange }, voice.env);
    player.play();
    player.next();
    expect(seen.last()).toEqual({ status: "playing", part: 1, sentence: 0 });
    expect(voice.queue).toEqual([]); // a beat after the cancel before the next queue
    voice.flush();
    expect(voice.queue[0]?.text).toBe("In api, Claude Code has finished.");
    voice.end();
    expect(seen.last()).toEqual({ status: "playing", part: 1, sentence: 1 });
    player.previous();
    expect(seen.last()).toEqual({ status: "playing", part: 1, sentence: 0 });
    player.previous();
    voice.flush();
    expect(seen.last()).toEqual({ status: "playing", part: 0, sentence: 0 });
    expect(voice.queue.map((line) => line.text)).toHaveLength(5); // the first beat's queue was dropped for the second's
    player.next();
    player.next();
    player.next();
    voice.flush();
    expect(seen.last()?.status).toBe("done");
    expect(voice.queue).toEqual([]);
  });

  it("moves while paused without speaking, and starts again from the top once done", () => {
    const voice = stand();
    const seen = watch();
    const player = createPlayer(PARTS, { onChange: seen.onChange }, voice.env);
    player.play();
    player.pause();
    player.next();
    expect(seen.last()).toEqual({ status: "paused", part: 1, sentence: 0 });
    expect(voice.queue).toEqual([]);
    player.next();
    player.next();
    expect(seen.last()?.status).toBe("done");
    player.play();
    expect(seen.last()).toEqual({ status: "playing", part: 0, sentence: 0 });
  });

  it("says why it stopped when the browser refuses, and hears nothing more from that queue", () => {
    const voice = stand();
    const seen = watch();
    const player = createPlayer(PARTS, { onChange: seen.onChange }, voice.env);
    player.play();
    voice.queue[0]!.onerror?.({ error: "not-allowed" });
    expect(seen.last()).toEqual({ status: "failed", part: 0, sentence: 0, error: "not-allowed" });
    expect(createPlayer(PARTS, { onChange: seen.onChange }, { ...voice.env, synth: () => null }).play()).toBeUndefined();
    expect(seen.last()?.error).toBe("no-voice");
  });

  it("speaks at the pitch and pace asked", () => {
    const voice = stand();
    createPlayer(PARTS.slice(0, 1), { pitch: 1.5, rate: 0.9, onChange: () => {} }, voice.env).play();
    expect(voice.queue.map((line) => [line.pitch, line.rate])).toEqual([
      [1.5, 0.9],
      [1.5, 0.9],
    ]);
  });

  it("is one at a time on the page: a new script stops the one before", () => {
    const voice = stand();
    const first = watch();
    speakScript(PARTS, { onChange: first.onChange }, voice.env);
    speakScript(PARTS.slice(2), { onChange: () => {} }, voice.env);
    expect(first.last()?.status).toBe("idle");
    expect(voice.queue.map((line) => line.text)).toEqual(["That's all for now."]);
  });
});

describe("priming", () => {
  it("speaks a silent space once, inside the gesture, and runs what waited for it", () => {
    const voice = stand();
    const ran: string[] = [];
    whenPrimed(() => ran.push("tell"));
    expect(ran).toEqual([]);
    primeSpeech(voice.env);
    expect(ran).toEqual(["tell"]);
    expect(voice.queue.map((line) => [line.text, line.volume])).toEqual([[" ", 0]]);
    primeSpeech(voice.env);
    expect(voice.queue).toHaveLength(1);
    whenPrimed(() => ran.push("again"));
    expect(ran).toEqual(["tell", "again"]);
  });
});

describe("isAppleTouch", () => {
  it("knows an iPhone, and an iPad that says it is a Mac", () => {
    expect(isAppleTouch("Mozilla/5.0 (iPhone; CPU iPhone OS 18_1 like Mac OS X)", 5)).toBe(true);
    expect(isAppleTouch("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)", 5)).toBe(true);
    expect(isAppleTouch("Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)", 0)).toBe(false);
    expect(isAppleTouch("Mozilla/5.0 (Linux; Android 15; Pixel 9)", 5)).toBe(false);
  });
});
