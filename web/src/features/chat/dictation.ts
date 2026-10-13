// Dictation through the browser's own speech recognition: on the device or its platform's service, never through remuda.

import { useEffect, useRef, useState } from "react";

type SpeechRecognitionLike = {
  lang: string;
  continuous: boolean;
  interimResults: boolean;
  onresult: ((e: { resultIndex: number; results: ArrayLike<{ isFinal: boolean; 0: { transcript: string } }> }) => void) | null;
  onend: (() => void) | null;
  onerror: (() => void) | null;
  start: () => void;
  stop: () => void;
};
type SpeechCtor = new () => SpeechRecognitionLike;
const speech = window as unknown as { SpeechRecognition?: SpeechCtor; webkitSpeechRecognition?: SpeechCtor };
const SPEECH = speech.SpeechRecognition ?? speech.webkitSpeechRecognition;

export function useDictation(onText: (text: string) => void) {
  const [listening, setListening] = useState(false);
  const rec = useRef<SpeechRecognitionLike | null>(null);
  const toggle = () => {
    if (!SPEECH) return;
    if (rec.current) {
      rec.current.stop();
      return;
    }
    const r = new SPEECH();
    r.lang = navigator.language || "en-US";
    r.continuous = true;
    r.interimResults = false;
    r.onresult = (e) => {
      for (let i = e.resultIndex; i < e.results.length; i++) {
        const res = e.results[i]!;
        if (res.isFinal) onText(res[0].transcript.trim());
      }
    };
    r.onend = () => {
      rec.current = null;
      setListening(false);
    };
    r.onerror = () => {
      rec.current = null;
      setListening(false);
    };
    rec.current = r;
    try {
      r.start();
      setListening(true);
    } catch {
      rec.current = null;
    }
  };
  // unmount: the handlers go before stop(), so nothing lands afterwards
  useEffect(
    () => () => {
      const r = rec.current;
      if (!r) return;
      r.onresult = r.onend = r.onerror = null;
      r.stop();
      rec.current = null;
    },
    [],
  );
  return { supported: !!SPEECH, listening, toggle };
}
