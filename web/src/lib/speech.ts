// The browser's own voice reading a script aloud: a script is parts (an agent each, say) of sentences, each in its
// language; the player queues every sentence from where it stands at once, so a page the browser freezes behind a
// locked screen keeps talking where it can, and pause, skip and back cancel the queue and start it again from a
// sentence. One player at a time on the page. Nothing passes through remuda. Safari on iOS speaks only after a gesture
// of the person's has spoken once: primeSpeech, called inside a click or a key, does that and runs what waited for it.

export type Sentence = { text: string; lang: string }; // lang: "en", "ru", or "" for the browser's own choice
export type Part = { key: string; title: string; sentences: Sentence[] };

export type Status = "idle" | "playing" | "paused" | "done" | "failed";
export type PlayState = { status: Status; part: number; sentence: number; error?: string };

export type VoiceLike = { name: string; lang: string; default?: boolean; localService?: boolean };
export type UtteranceLike = {
  text: string;
  lang: string;
  voice: VoiceLike | null;
  pitch: number;
  rate: number;
  volume: number;
  onstart: (() => void) | null;
  onend: (() => void) | null;
  onerror: ((event: { error?: string }) => void) | null;
};
export type SynthLike = { speak(line: UtteranceLike): void; cancel(): void; getVoices(): VoiceLike[]; speaking: boolean; pending: boolean };

// Env is how the player reaches a voice: the browser's, or a stand-in in a test; later runs what follows a cancel
export type Env = { synth: () => SynthLike | null; utter: (text: string) => UtteranceLike; later: (fn: () => void) => void };

// AFTER_CANCEL_MS is the beat between a cancel and the next queue: Chrome, handed a line the moment it cancelled, may
// drop it with the ones it was cancelling
const AFTER_CANCEL_MS = 80;

const browserEnv: Env = {
  synth: () => (typeof window !== "undefined" && "speechSynthesis" in window ? (window.speechSynthesis as unknown as SynthLike) : null),
  utter: (text) => new SpeechSynthesisUtterance(text) as unknown as UtteranceLike,
  later: (fn) => setTimeout(fn, AFTER_CANCEL_MS),
};

// speechSupported says whether this browser has a voice to speak with
export const speechSupported = (): boolean => typeof window !== "undefined" && "speechSynthesis" in window && "SpeechSynthesisUtterance" in window;

const langOfVoice = (voice: VoiceLike) => voice.lang.toLowerCase().replace("_", "-");

// pickVoice is the voice for a sentence: the one the person picked when it speaks the sentence's language (or the
// sentence names none), else the best of that language — the browser's default for it, then one on the device —
// else the one picked; none leaves the choice to the browser
export function pickVoice(voices: VoiceLike[], lang: string, picked?: string): VoiceLike | undefined {
  const chosen = picked ? voices.find((voice) => voice.name === picked) : undefined;
  if (!lang) return chosen;
  const speaks = (voice: VoiceLike) => langOfVoice(voice).startsWith(lang.toLowerCase());
  if (chosen && speaks(chosen)) return chosen;
  const own = voices.filter(speaks);
  return own.find((voice) => voice.default) ?? own.find((voice) => voice.localService) ?? own[0] ?? chosen;
}

export type PlayerOptions = { voiceName?: string; pitch?: number; rate?: number; onChange: (state: PlayState) => void };
export type Player = { state(): PlayState; play(): void; pause(): void; toggle(): void; next(): void; previous(): void; stop(): void };

// OUR_ERRORS are what an utterance says when the queue it was in was cancelled on purpose
const OUR_ERRORS = new Set(["interrupted", "canceled"]);

// createPlayer reads parts aloud; onChange hears every move, the sentence shown being the one now said
export function createPlayer(parts: Part[], options: PlayerOptions, env: Env = browserEnv): Player {
  let state: PlayState = { status: "idle", part: 0, sentence: 0 };
  let generation = 0;
  let held: UtteranceLike[] = []; // kept, since a browser may forget an utterance nobody holds before it ends
  const set = (next: PlayState) => {
    state = next;
    options.onChange(state);
  };
  // halt drops the queue, and says whether the voice had any of it left to cancel
  const halt = (): boolean => {
    generation++;
    held = [];
    const synth = env.synth();
    if (!synth || !(synth.speaking || synth.pending)) return false;
    synth.cancel();
    return true;
  };
  const speakFrom = (part: number, sentence: number) => {
    const cancelled = halt();
    const synth = env.synth();
    if (!synth) return set({ status: "failed", part, sentence, error: "no-voice" });
    const mine = generation;
    const voices = synth.getVoices();
    const queue: { part: number; sentence: number; line: Sentence }[] = [];
    for (let p = part; p < parts.length; p++) parts[p]!.sentences.forEach((line, s) => (p > part || s >= sentence) && queue.push({ part: p, sentence: s, line }));
    if (queue.length === 0) return set({ status: "done", part: Math.max(0, parts.length - 1), sentence: 0 });
    set({ status: "playing", part: queue[0]!.part, sentence: queue[0]!.sentence });
    const lines = queue.map((at, i) => {
      const utterance = env.utter(at.line.text);
      const voice = pickVoice(voices, at.line.lang, options.voiceName);
      if (voice) utterance.voice = voice;
      if (at.line.lang) utterance.lang = voice?.lang ?? at.line.lang;
      utterance.pitch = options.pitch ?? 1;
      utterance.rate = options.rate ?? 1;
      utterance.onstart = () => mine === generation && state.status === "playing" && set({ status: "playing", part: at.part, sentence: at.sentence });
      utterance.onend = () => mine === generation && i === queue.length - 1 && set({ status: "done", part: at.part, sentence: at.sentence });
      utterance.onerror = (event) => {
        if (mine !== generation || OUR_ERRORS.has(event.error ?? "")) return;
        halt();
        set({ status: "failed", part: at.part, sentence: at.sentence, error: event.error || "synthesis-failed" });
      };
      return utterance;
    });
    const speak = () => {
      if (mine !== generation) return;
      held = lines;
      for (const line of lines) synth.speak(line);
    };
    if (cancelled) env.later(speak);
    else speak();
  };
  const player: Player = {
    state: () => state,
    play() {
      if (state.status === "playing") return;
      if (state.status === "done" || state.status === "failed") speakFrom(0, 0);
      else speakFrom(state.part, state.sentence);
    },
    pause() {
      if (state.status !== "playing") return;
      halt();
      set({ ...state, status: "paused" });
    },
    toggle() {
      if (state.status === "playing") player.pause();
      else player.play();
    },
    next() {
      if (state.part + 1 >= parts.length) {
        halt();
        return set({ status: "done", part: state.part, sentence: state.sentence });
      }
      if (state.status === "playing") speakFrom(state.part + 1, 0);
      else set({ status: "paused", part: state.part + 1, sentence: 0 });
    },
    previous() {
      const to = state.sentence > 0 ? state.part : Math.max(0, state.part - 1);
      if (state.status === "playing") speakFrom(to, 0);
      else set({ status: "paused", part: to, sentence: 0 });
    },
    stop() {
      halt();
      set({ status: "idle", part: 0, sentence: 0 });
    },
  };
  return player;
}

// nowSaid is the sentence a state stands on, and its part's title
export function nowSaid(parts: Part[], state: PlayState | null): { title: string; text: string } {
  const part = state ? parts[state.part] : undefined;
  return { title: part?.title ?? "", text: (state && part?.sentences[state.sentence]?.text) || "" };
}

let current: Player | null = null;

// speakScript reads a script with the one player of the page, stopping whatever it was reading before
export function speakScript(parts: Part[], options: PlayerOptions, env: Env = browserEnv): Player {
  current?.stop();
  const player = createPlayer(parts, options, env);
  current = player;
  player.play();
  return player;
}

// stopSpeaking stops the page's player, when it is this one (or any, without one named)
export function stopSpeaking(player?: Player | null): void {
  if (!current || (player && player !== current)) return;
  current.stop();
  current = null;
}

let primed = false;
let waiting: (() => void)[] = [];

// primeSpeech, inside a click or a key, speaks a silent space once so Safari on iOS lets the page speak later, then
// runs what waited for it
export function primeSpeech(env: Env = browserEnv): void {
  if (!primed) {
    const synth = env.synth();
    if (synth) {
      const line = env.utter(" ");
      line.volume = 0;
      synth.speak(line);
    }
  }
  markPrimed();
}

// markPrimed notes that the page has spoken inside a gesture of the person's — a player started from a click — and runs
// what waited for it
export function markPrimed(): void {
  primed = true;
  const run = waiting;
  waiting = [];
  for (const fn of run) fn();
}

// whenPrimed runs fn at once once the page has primed its voice, else at the next click or key
export function whenPrimed(fn: () => void): void {
  if (primed) fn();
  else waiting.push(fn);
}

// armPriming primes the voice at the next tap, click or key on target; the function it returns stops listening
export function armPriming(target: EventTarget): () => void {
  const prime = () => primeSpeech();
  for (const type of ["pointerup", "click", "keydown"]) target.addEventListener(type, prime, { capture: true });
  return () => {
    for (const type of ["pointerup", "click", "keydown"]) target.removeEventListener(type, prime, { capture: true });
  };
}

// resetPriming forgets the priming, for a test
export function resetPriming(): void {
  primed = false;
  waiting = [];
  current = null;
}

// isAppleTouch says whether the page runs on an iPhone or an iPad, whose Safari stops speaking when the screen locks
export function isAppleTouch(userAgent: string, touchPoints: number): boolean {
  return /iPhone|iPad|iPod/.test(userAgent) || (/Macintosh/.test(userAgent) && touchPoints > 1);
}
