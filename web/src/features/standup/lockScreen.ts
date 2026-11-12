// The phone's lock screen while the day is read aloud: the Media Session API names what is said and takes its play,
// pause, next and previous buttons — but a browser shows it only for a page that plays media, and speech is none, so a
// silent sound made here (never fetched) loops under the voice while it reads. Where the browser has no Media Session,
// nothing shows and nothing breaks.

export type LockActions = { play: () => void; pause: () => void; next: () => void; previous: () => void; stop: () => void };
export type LockMeta = { title: string; artist: string; album: string };

// SILENCE_S is how long the silent loop runs before it starts again: Chrome names a media session only past five seconds
const SILENCE_S = 10;

// silentWav is a WAV file of silence: mono, 8-bit, at rate samples a second
export function silentWav(seconds: number, rate = 8000): Uint8Array {
  const samples = Math.round(seconds * rate);
  const bytes = new Uint8Array(44 + samples);
  const view = new DataView(bytes.buffer);
  const text = (at: number, word: string) => [...word].forEach((char, i) => view.setUint8(at + i, char.charCodeAt(0)));
  text(0, "RIFF");
  view.setUint32(4, 36 + samples, true);
  text(8, "WAVE");
  text(12, "fmt ");
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true); // PCM
  view.setUint16(22, 1, true); // mono
  view.setUint32(24, rate, true);
  view.setUint32(28, rate, true); // bytes a second
  view.setUint16(32, 1, true); // bytes a frame
  view.setUint16(34, 8, true); // bits a sample
  text(36, "data");
  view.setUint32(40, samples, true);
  bytes.fill(128, 44); // an unsigned 8-bit sample's silence is its middle
  return bytes;
}

// lockTitle is a sentence cut for the lock screen's line, at a word
export function lockTitle(sentence: string, max = 80): string {
  if (sentence.length <= max) return sentence;
  const cut = sentence.slice(0, max - 1);
  const space = cut.lastIndexOf(" ");
  return `${space > max / 2 ? cut.slice(0, space) : cut}…`;
}

let audio: HTMLAudioElement | null = null;
let source = "";

const session = (): MediaSession | null => (typeof navigator !== "undefined" && "mediaSession" in navigator ? navigator.mediaSession : null);

// holdLockScreen starts the silent loop; inside a click or a key, the gesture a browser waits for before it plays
export function holdLockScreen(): void {
  if (typeof Audio === "undefined" || !session()) return;
  try {
    if (!audio) {
      source = URL.createObjectURL(new Blob([silentWav(SILENCE_S) as BlobPart], { type: "audio/wav" }));
      audio = new Audio(source);
      audio.loop = true;
    }
    void audio.play().catch(() => {});
  } catch {
    audio = null;
  }
}

const ACTIONS: [MediaSessionAction, keyof LockActions][] = [
  ["play", "play"],
  ["pause", "pause"],
  ["nexttrack", "next"],
  ["previoustrack", "previous"],
  ["stop", "stop"],
];

// handle sets one button's handler; a browser without that button throws, and keeps the others
function handle(media: MediaSession, action: MediaSessionAction, fn: (() => void) | null) {
  try {
    media.setActionHandler(action, fn);
  } catch {
    // not this browser's button
  }
}

// showOnLockScreen names what is said now and hands the lock screen's buttons to actions
export function showOnLockScreen(meta: LockMeta, playing: boolean, actions: LockActions): void {
  const media = session();
  if (!media) return;
  try {
    media.metadata = new MediaMetadata({ title: lockTitle(meta.title), artist: meta.artist, album: meta.album, artwork: [{ src: "/icons/icon-512.png", sizes: "512x512", type: "image/png" }] });
    media.playbackState = playing ? "playing" : "paused";
  } catch {
    // a browser that names nothing still takes the buttons
  }
  for (const [action, name] of ACTIONS) handle(media, action, () => actions[name]());
  if (!audio) return;
  if (playing && audio.paused) void audio.play().catch(() => {});
  if (!playing && !audio.paused) audio.pause();
}

// releaseLockScreen stops the loop and leaves the lock screen as the page found it
export function releaseLockScreen(): void {
  const media = session();
  if (media) {
    try {
      media.metadata = null;
      media.playbackState = "none";
    } catch {
      // nothing to undo
    }
    for (const [action] of ACTIONS) handle(media, action, null);
  }
  audio?.pause();
  audio = null;
  if (source) URL.revokeObjectURL(source);
  source = "";
}
