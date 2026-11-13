// ScreenTerminal is the terminal of a pane with no attach — a Windows machine's, where herdr has no terminal attach: the
// pane's screen read again and again (useLiveScreen), drawn as the Wall's screen view draws it at the size that fits
// the pane's columns, and typed into through a hidden field (useTyping). Text on it selects and copies as text; a
// selection holds the screen still. It never attaches and never resizes the pane. The find over the scrollback marks
// what it found on the screen, as xterm's search does in the live terminal.

import { useEffect, useLayoutEffect, useMemo, useRef, useState, type RefObject } from "react";
import { useFlavor } from "@/app/theme";
import { isTouch } from "@/lib/prefs";
import { ansiPalette } from "@/lib/theme";
import { Icon, Notice, Tip } from "@/ui";
import { screenTerminalWords } from "@/features/roster/system";
import { parseAnsi } from "../ansi";
import { scrollPane } from "../api";
import { screenFont } from "../engine/geometry";
import type { Modifier, TermApi } from "../engine/types";
import { ScreenLines, useFitFont, widthOf } from "../Screen";
import { markLines } from "../timelapse/mark";
import { useLiveScreen } from "./useLiveScreen";
import { selectedIn, useTyping } from "./useTyping";
import { strokeOf } from "./strokes";
import { t } from "@/lib/i18n";

// AT_BOTTOM is how close to the end the screen counts as followed: a new screen keeps the bottom rows in view
const AT_BOTTOM = 24;

export function ScreenTerminal({ machineId, paneId, cols, host, active, fontSize, modifier, onModifierUsed, onReady }: {
  machineId: string;
  paneId: string;
  cols: number; // the pane's own grid, 0 when unknown
  host: string; // the machine as `herdr --remote` reaches it, for the line saying where a live terminal is
  active: boolean; // on screen: hidden, nothing is read
  fontSize: number; // the text size picked: the screen is drawn no larger
  modifier: Modifier;
  onModifierUsed: () => void;
  onReady: (api: TermApi | null) => void;
}) {
  const box = useRef<HTMLDivElement>(null);
  const field = useRef<HTMLTextAreaElement>(null);
  const [typing, setTyping] = useState(false);
  const held = useSelectionHeld(box);
  const screen = useLiveScreen(machineId, paneId, { active, fast: typing, held });
  const keyboard = useTyping({ machineId, paneId, field, screen: box, modifier, onModifierUsed, onSent: screen.afterSend });
  const [found, setFound] = useState<{ query: string; row: number } | null>(null);

  const flavor = useFlavor();
  const palette = useMemo(() => ansiPalette(flavor), [flavor]);
  const lines = useMemo(() => (screen.text == null ? [] : parseAnsi(screen.text, palette)), [screen.text, palette]);
  const shown = useMemo(() => (found ? markLines(lines, found.query) : lines), [lines, found]);
  const font = useFitFont(box, widthOf(cols, lines), (avail, columns, cell, base) => screenFont(avail, columns, cell, base, fontSize));

  const following = useRef(true);
  useLayoutEffect(() => {
    const el = box.current;
    if (el && following.current) el.scrollTop = el.scrollHeight;
  }, [shown, font]);

  const live = useRef({ onReady, push: keyboard.push, focus: keyboard.focusField, reload: screen.reload });
  live.current = { onReady, push: keyboard.push, focus: keyboard.focusField, reload: screen.reload };
  useEffect(() => {
    // the find may have scrolled the pane's viewport up; with no wheel of its own here, the screen goes back to the bottom with it
    let searched = false;
    const api: TermApi = {
      input: (seq) => live.current.push(strokeOf(seq)),
      appCursor: () => false,
      focus: () => live.current.focus(),
      search: {
        show: (query, row) => {
          searched = true;
          setFound({ query, row });
          live.current.reload();
        },
        clear: () => {
          setFound(null);
          if (!searched) return;
          searched = false;
          scrollPane(machineId, paneId, 0).then(live.current.reload, () => {});
        },
      },
    };
    live.current.onReady(api);
    return () => live.current.onReady(null);
  }, [machineId, paneId]);

  // on screen with a keyboard, typing goes straight in; a touch screen's keyboard waits for a tap
  useEffect(() => {
    if (active && !isTouch) keyboard.focusField();
  }, [active]); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div className={`screen-term${typing ? " is-typing" : ""}`} tabIndex={-1} onKeyDown={keyboard.onKeyDown}>
      <div
        className="screen screen-term-screen"
        ref={box}
        style={{ fontSize: font }}
        onClick={() => !selectedIn(box.current) && keyboard.focusField()}
        onScroll={(e) => {
          const el = e.currentTarget;
          following.current = el.scrollHeight - el.scrollTop - el.clientHeight < AT_BOTTOM;
        }}
      >
        {screen.text == null && !screen.error && <div className="empty chat-empty">{t("Loading…")}</div>}
        {screen.error && (
          <Notice tone="error" className="chat-notice">
            {screen.error}
          </Notice>
        )}
        <ScreenLines lines={shown} current={found ? found.row : -1} />
      </div>
      <textarea
        ref={field}
        className="screen-term-field"
        aria-label={t("Type into the pane")}
        rows={1}
        autoCapitalize="off"
        autoComplete="off"
        autoCorrect="off"
        spellCheck={false}
        onFocus={() => setTyping(true)}
        onBlur={() => setTyping(false)}
        {...keyboard.fieldHandlers}
      />
      <div className="screen-term-note">
        <Tip text={screenTerminalWords(host)}>
          <button type="button" className="icon-btn sm" aria-label={t("Why a screen and not a live terminal")}>
            <Icon name="info" size={12} />
          </button>
        </Tip>
        <span>{t("The pane's screen, typed into: herdr has no live terminal on Windows")}</span>
      </div>
    </div>
  );
}

// useSelectionHeld says a selection lies on the screen: the reads wait, so the rows under it stay put while it is copied
function useSelectionHeld(box: RefObject<HTMLDivElement | null>): boolean {
  const [held, setHeld] = useState(false);
  useEffect(() => {
    const doc = box.current?.ownerDocument ?? document;
    const check = () => setHeld(selectedIn(box.current));
    doc.addEventListener("selectionchange", check);
    return () => doc.removeEventListener("selectionchange", check);
  }, [box]);
  return held;
}
