// The browser's own voice for the roster: what an agent answered when it finishes, that one waits, a watchdog note —
// while the switch is on and the page is in front. Nothing passes through remuda; the words are the roster's.

import { useEffect, useRef, useState } from "react";
import { useRoster } from "@/features/roster/useRoster";
import { usePref } from "@/lib/prefs";
import { DEFAULT_VOICE, linesToSay, type VoicePrefs } from "./voice";

export const voiceSupported = typeof window !== "undefined" && "speechSynthesis" in window && "SpeechSynthesisUtterance" in window;

// speak queues one line after whatever is being said, in the voice picked, else one of the line's language, else the default
export function speak(text: string, lang: string, voiceName?: string): void {
  if (!voiceSupported) return;
  const line = new SpeechSynthesisUtterance(text);
  const voices = speechSynthesis.getVoices();
  const picked = voiceName ? voices.find((v) => v.name === voiceName) : undefined;
  if (picked) line.voice = picked;
  else if (lang) {
    const spoken = voices.find((v) => v.lang.toLowerCase().startsWith(lang));
    if (spoken) line.voice = spoken;
    else line.lang = lang;
  }
  speechSynthesis.speak(line);
}

// useVoice reads the roster's changes aloud: the first roster only seeds what is already there, so a reload says nothing
export function useVoice(): void {
  const { machines } = useRoster();
  const [prefs] = usePref<VoicePrefs>("alerts.voice", DEFAULT_VOICE);
  const heard = useRef<Set<string> | null>(null);
  useEffect(() => {
    const first = heard.current === null;
    const { lines, keys } = linesToSay(machines, heard.current ?? new Set());
    heard.current = keys;
    if (first || !prefs.on || !voiceSupported || document.visibilityState !== "visible") return;
    for (const line of lines) speak(line.text, line.lang, prefs.voice);
  }, [machines, prefs.on, prefs.voice]);
  useEffect(() => {
    if (!prefs.on && voiceSupported) speechSynthesis.cancel();
  }, [prefs.on]);
}

// useVoices lists the browser's voices, which Chrome hands over a moment after the page loads
export function useVoices(): SpeechSynthesisVoice[] {
  const [voices, setVoices] = useState<SpeechSynthesisVoice[]>(() => (voiceSupported ? speechSynthesis.getVoices() : []));
  useEffect(() => {
    if (!voiceSupported) return;
    const refresh = () => setVoices(speechSynthesis.getVoices());
    refresh();
    speechSynthesis.addEventListener("voiceschanged", refresh);
    return () => speechSynthesis.removeEventListener("voiceschanged", refresh);
  }, []);
  return voices;
}
