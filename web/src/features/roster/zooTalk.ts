// What a creature of the zoo says of its day when asked: a sentence or two in its own voice, from its agent's recap
// since midnight and how it stands now, and the pitch and pace its species speaks at — the same voice its chirps have
// (zooLife's voiceOf), high for a gemini, low and a little slow for a robot. The pure part, tested.

import type { Recap } from "@/features/standup/api";
import { quoted, ranOf, say } from "@/features/standup/spoken";
import { commandsWords, filesWords, timesWords } from "@/features/standup/speakable";
import { quiet } from "@/features/standup/standup";
import { t } from "@/lib/i18n";
import type { Sentence } from "@/lib/speech";
import type { Pose } from "./zoo";
import { voiceOf } from "./zooLife";
import type { Species } from "./zooWear";

// todayWords is what it did since midnight, in one sentence
function todayWords(recap: Recap | null): string {
  if (!recap || quiet(recap)) return t("I haven't done anything yet today.");
  if (recap.answers > 0) {
    const times = timesWords(recap.answers);
    if (recap.edited.length) return t("Today I answered {times} and edited {files}.", { times, files: filesWords(recap.edited) });
    const ran = ranOf(recap);
    if (ran) return t("Today I answered {times} and ran {commands}.", { times, commands: commandsWords(ran) });
    return t("Today I answered {times}.", { times });
  }
  if (recap.prompts > 0) return t("Today I'm still on my first answer.");
  return t("Today I've been busy with my tools.");
}

// nowWords is how it stands, by its pose; its last answer quoted once it rests or has finished
function nowWords(pose: Pose, said: string): Sentence[] {
  switch (pose) {
    case "asking":
      return [say(t("Now I'm waiting for your answer."))];
    case "worried":
      return [say(t("Something is off here: have a look."))];
    case "drained":
      return [say(t("I've hit my plan limit and wait for it to reset."))];
    case "busy":
      return [say(t("I'm still at it."))];
    case "dancing":
    case "asleep":
      if (said) return [say(t("My last answer:")), quoted(said)];
      return [say(pose === "dancing" ? t("I've just finished.") : t("Now I'm resting."))];
    default:
      return [];
  }
}

// creatureTalk is what a creature says when asked about its day; said is its agent's last answer as it may be said aloud
export function creatureTalk(recap: Recap | null, pose: Pose, said = ""): Sentence[] {
  return [say(todayWords(recap)), ...nowWords(pose, said)];
}

// speechOf is the pitch and pace a species speaks at, from its chirp: its pitch against a middle C's octave, and the
// robot's square wave and the blob's saw a little slower, the round ones a little quicker
export function speechOf(species: Species): { pitch: number; rate: number } {
  const { wave, base } = voiceOf(species);
  const pitch = Math.min(1.6, Math.max(0.6, Math.round((base / 523) * 100) / 100));
  const rate = wave === "square" ? 0.92 : wave === "sawtooth" ? 0.9 : wave === "triangle" ? 1.08 : 1.04;
  return { pitch, rate };
}
