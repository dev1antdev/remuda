// useListen is a script read aloud from a panel: play and pause, the next part and back, stop; in the voice the alerts
// panel picked, else one of the sentence's language; the phone's lock screen naming what is said and taking its
// buttons. Closing the panel stops it.

import { useCallback, useEffect, useRef, useState } from "react";
import { DEFAULT_VOICE, type VoicePrefs } from "@/features/alerts/voice";
import { usePref } from "@/lib/prefs";
import { nowSaid, speakScript, stopSpeaking, type Part, type PlayState, type Player } from "@/lib/speech";
import { holdLockScreen, releaseLockScreen, showOnLockScreen, type LockActions } from "./lockScreen";

export type Listening = {
  state: PlayState | null; // null before the first Listen and after a stop
  parts: Part[]; // the script being read
  start: (parts: Part[]) => void;
  toggle: () => void;
  next: () => void;
  previous: () => void;
  stop: () => void;
};

export function useListen(album: string): Listening {
  const [prefs] = usePref<VoicePrefs>("alerts.voice", DEFAULT_VOICE);
  const [state, setState] = useState<PlayState | null>(null);
  const [parts, setParts] = useState<Part[]>([]);
  const player = useRef<Player | null>(null);
  const script = useRef<Part[]>([]);
  const lock = useRef<LockActions>({ play: () => {}, pause: () => {}, next: () => {}, previous: () => {}, stop: () => {} });

  const stop = useCallback(() => {
    stopSpeaking(player.current);
    player.current = null;
    releaseLockScreen();
    setState(null);
  }, []);
  lock.current = { play: () => player.current?.play(), pause: () => player.current?.pause(), next: () => player.current?.next(), previous: () => player.current?.previous(), stop };

  const start = useCallback(
    (next: Part[]) => {
      holdLockScreen();
      script.current = next;
      setParts(next);
      player.current = speakScript(next, {
        voiceName: prefs.voice,
        onChange: (moved) => {
          setState(moved);
          const { title, text } = nowSaid(script.current, moved);
          if (moved.status !== "idle") showOnLockScreen({ title: text || title, artist: title, album }, moved.status === "playing", lock.current);
        },
      });
    },
    [prefs.voice, album],
  );

  useEffect(() => stop, [stop]);
  return {
    state,
    parts,
    start,
    toggle: () => player.current?.toggle(),
    next: () => player.current?.next(),
    previous: () => player.current?.previous(),
    stop,
  };
}
