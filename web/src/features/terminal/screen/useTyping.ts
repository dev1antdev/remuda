// useTyping is the screen terminal's keyboard: a hidden field takes what is typed — the characters as text, through an
// input method or a paste too — and a keydown on it or on the screen takes the keys herdr names; everything goes out
// through one sender, coalesced, in order. The key bar's one-shot Ctrl or Alt is spent on the next stroke.

import { useCallback, useEffect, useMemo, useRef, type ClipboardEvent, type KeyboardEvent, type RefObject } from "react";
import { actions } from "@/app/store";
import { errMsg } from "@/lib/text";
import { sendInput } from "../api";
import { isFindKey, MAC } from "../engine/keys";
import type { Modifier } from "../engine/types";
import { createSender } from "./sender";
import { keyOf, withModifier, type Stroke } from "./strokes";

type Options = {
  machineId: string;
  paneId: string;
  field: RefObject<HTMLTextAreaElement | null>;
  screen: RefObject<HTMLElement | null>;
  modifier: Modifier;
  onModifierUsed: () => void;
  onSent: () => void;
};

// selectedIn says a selection of the page's lies in the element: a copy, not a key for the pane
export function selectedIn(el: HTMLElement | null): boolean {
  const selection = el?.ownerDocument.getSelection();
  return !!el && !!selection && !selection.isCollapsed && !!selection.anchorNode && el.contains(selection.anchorNode);
}

export function useTyping({ machineId, paneId, field, screen, modifier, onModifierUsed, onSent }: Options) {
  const live = useRef({ modifier, onModifierUsed, onSent });
  live.current = { modifier, onModifierUsed, onSent };
  const sender = useMemo(
    () =>
      createSender((batch) => sendInput(machineId, paneId, batch), {
        sent: () => live.current.onSent(),
        failed: (error) => actions.notify(errMsg(error), { error: true }),
      }),
    [machineId, paneId],
  );
  const push = useCallback(
    (stroke: Stroke) => {
      const { stroke: sent, used } = withModifier(stroke, live.current.modifier);
      if (used) live.current.onModifierUsed();
      sender.push(sent);
    },
    [sender],
  );
  const composing = useRef(false);
  // takeTyped sends what the field holds, unless an input method is still composing it
  const takeTyped = useCallback(() => {
    const el = field.current;
    if (!el || composing.current || !el.value) return;
    push({ text: el.value });
    el.value = "";
  }, [field, push]);
  const focusField = useCallback(() => field.current?.focus({ preventScroll: true }), [field]);

  const onKeyDown = (e: KeyboardEvent<HTMLElement>) => {
    const press = e.nativeEvent;
    if (isFindKey(press, MAC)) return;
    if (e.code === "KeyC" && (MAC ? e.metaKey : e.ctrlKey) && selectedIn(screen.current)) return; // the browser copies
    const key = keyOf(press, MAC);
    if (key) {
      e.preventDefault();
      takeTyped();
      push({ key });
      if (e.target !== field.current) focusField();
      return;
    }
    if (e.key.length === 1 && !e.ctrlKey && !e.metaKey && e.target !== field.current) focusField(); // the character lands in the field
  };

  // a phone's keyboard sends some keys as edits of the field rather than as keydowns
  useEffect(() => {
    const el = field.current;
    if (!el) return;
    const onBeforeInput = (e: InputEvent) => {
      if (e.inputType === "deleteContentBackward" && !el.value) {
        e.preventDefault();
        push({ key: "backspace" });
      } else if (e.inputType === "insertLineBreak" || e.inputType === "insertParagraph") {
        e.preventDefault();
        takeTyped();
        push({ key: "enter" });
      }
    };
    el.addEventListener("beforeinput", onBeforeInput);
    return () => el.removeEventListener("beforeinput", onBeforeInput);
  }, [field, push, takeTyped]);

  const fieldHandlers = {
    onInput: takeTyped,
    onCompositionStart: () => {
      composing.current = true;
    },
    onCompositionEnd: () => {
      composing.current = false;
      takeTyped();
    },
    onPaste: (e: ClipboardEvent<HTMLTextAreaElement>) => {
      const text = e.clipboardData.getData("text/plain");
      e.preventDefault();
      if (text) push({ text, paste: true });
    },
  };
  return { push, onKeyDown, fieldHandlers, focusField };
}
