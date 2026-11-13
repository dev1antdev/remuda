// The device's pick of a sound for an alert on the page: the alerts panel sets it, the page's alerts ring by it. A
// device that had the chime switched on before the pick existed starts on the chime.

import { readPref, usePref } from "@/lib/prefs";
import { choiceOf, LEGACY_SOUND_KEY, SOUNDS_KEY, type SoundChoice } from "./sounds";

export function useSoundChoice(): [SoundChoice, (choice: SoundChoice) => void] {
  const [stored, setChoice] = usePref<string>(SOUNDS_KEY, choiceOf(undefined, readPref<boolean>(LEGACY_SOUND_KEY, false)));
  return [choiceOf(stored, false), setChoice];
}
