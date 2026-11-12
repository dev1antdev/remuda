// useTell has a creature say what its agent did today: its recap since this device's midnight (one read), a sentence or
// two in its species' pitch and pace, the sentence being said shown in its bubble. One creature at a time; asking the
// same one again stops it. The voice is the alerts panel's pick, else one of the sentence's language.

import { useCallback, useEffect, useRef, useState } from "react";
import { DEFAULT_VOICE, type VoicePrefs } from "@/features/alerts/voice";
import type { Tile } from "@/features/roster/wall";
import type { Pose } from "@/features/roster/zoo";
import { creatureTalk, speechOf } from "@/features/roster/zooTalk";
import { speciesOf } from "@/features/roster/zooWear";
import { fetchRecap } from "@/features/standup/api";
import { usePref } from "@/lib/prefs";
import { armPriming, speakScript, stopSpeaking, whenPrimed, type Player } from "@/lib/speech";

// Telling is the creature speaking and the sentence in its bubble ("…" while its recap is read)
export type Telling = { key: string; text: string } | null;

// LINGER_MS is how long the last sentence stays in the bubble once the voice is done
const LINGER_MS = 1800;

const midnight = () => {
  const at = new Date();
  at.setHours(0, 0, 0, 0);
  return at.toISOString();
};

export function useTell(quote: (text: string) => string): { telling: Telling; tell: (tile: Tile, pose: Pose) => void } {
  const [prefs] = usePref<VoicePrefs>("alerts.voice", DEFAULT_VOICE);
  const [telling, setTelling] = useState<Telling>(null);
  const shown = useRef<Telling>(null);
  const player = useRef<Player | null>(null);
  const linger = useRef<ReturnType<typeof setTimeout> | undefined>(undefined);
  const asked = useRef(0);
  const show = (next: Telling) => {
    shown.current = next;
    setTelling(next);
  };
  useEffect(() => armPriming(window), []);
  useEffect(
    () => () => {
      asked.current++;
      clearTimeout(linger.current);
      stopSpeaking(player.current);
    },
    [],
  );
  const tell = useCallback(
    (tile: Tile, pose: Pose) => {
      const n = ++asked.current;
      clearTimeout(linger.current);
      stopSpeaking(player.current);
      if (shown.current?.key === tile.key) return show(null);
      show({ key: tile.key, text: "…" });
      const said = tile.pane.said ? quote(tile.pane.said) : "";
      const { pitch, rate } = speechOf(speciesOf(tile.pane.agent));
      void fetchRecap(tile.machine.id, tile.pane.id, null, "", midnight())
        .then(({ recap }) => recap)
        .catch(() => null)
        .then((recap) => {
          if (n !== asked.current) return;
          const sentences = creatureTalk(recap, pose, said);
          const over = (wait: number) => {
            linger.current = setTimeout(() => n === asked.current && show(null), wait);
          };
          whenPrimed(() => {
            if (n !== asked.current) return;
            player.current = speakScript([{ key: tile.key, title: tile.key, sentences }], {
              voiceName: prefs.voice,
              pitch,
              rate,
              onChange: (state) => {
                if (n !== asked.current) return;
                if (state.status === "playing") show({ key: tile.key, text: sentences[state.sentence]?.text ?? "" });
                else if (state.status === "done") over(LINGER_MS);
                else over(0);
              },
            });
          });
        });
    },
    [prefs.voice, quote],
  );
  return { telling, tell };
}
