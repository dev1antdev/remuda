import type { ReactNode } from "react";
import Icon from "./icons";
import type { Modifier, TermApi } from "./Terminal";

type Key = { id: string; label: ReactNode; title?: string; wide?: boolean } & ({ seq: string } | { arrow: "A" | "B" | "C" | "D" } | { mod: Exclude<Modifier, null> });

// Row 1: what a phone keyboard lacks and an agent TUI needs constantly.
// Row 2: the rest of a keyboard. Sequences are what a terminal would send;
// arrows follow DECCKM, so they match whatever the TUI asked for.
const ROW1: Key[] = [
  { id: "Esc", label: "Esc", seq: "\x1b" },
  { id: "Tab", label: "Tab", seq: "\t" },
  { id: "Up", label: <Icon name="chevron-up" size={14} />, arrow: "A" },
  { id: "Down", label: <Icon name="chevron-down" size={14} />, arrow: "B" },
  { id: "Left", label: <Icon name="chevron-left" size={14} />, arrow: "D" },
  { id: "Right", label: <Icon name="chevron-right" size={14} />, arrow: "C" },
  { id: "Newline", label: <Icon name="corner-down-left" size={14} />, seq: "\x1b\r", title: "Newline without sending (Alt+Enter)" },
  { id: "Enter", label: "Enter", seq: "\r", wide: true },
  { id: "Ctrl", label: "Ctrl", mod: "ctrl", title: "Hold Ctrl for the next key" },
  { id: "^C", label: "^C", seq: "\x03", title: "Interrupt" },
];

const ROW2: Key[] = [
  { id: "Alt", label: "Alt", mod: "alt", title: "Hold Alt for the next key" },
  { id: "Shift+Tab", label: "⇤", seq: "\x1b[Z", title: "Shift+Tab" },
  { id: "Home", label: "Home", seq: "\x1b[H" },
  { id: "End", label: "End", seq: "\x1b[F" },
  { id: "PgUp", label: "PgUp", seq: "\x1b[5~" },
  { id: "PgDn", label: "PgDn", seq: "\x1b[6~" },
  { id: "Backspace", label: "⌫", seq: "\x7f", title: "Backspace" },
  { id: "Del", label: "Del", seq: "\x1b[3~" },
  { id: "^D", label: "^D", seq: "\x04" },
  { id: "^Z", label: "^Z", seq: "\x1a" },
  { id: "^L", label: "^L", seq: "\x0c", title: "Clear screen" },
  { id: "^U", label: "^U", seq: "\x15", title: "Clear line" },
  { id: "^R", label: "^R", seq: "\x12", title: "Search history" },
  { id: "^A", label: "^A", seq: "\x01" },
  { id: "^E", label: "^E", seq: "\x05" },
];

// Key bar for the terminal lens. Every key goes through the live terminal's
// own input path, exactly like a keystroke — no second send path, no server
// key grammar; the terminal applies the one-shot modifier itself. Pointer
// events are prevented so the terminal keeps focus and the soft keyboard
// stays up.
export default function KeyBar({
  term,
  modifier,
  setModifier,
}: {
  term: TermApi | null;
  modifier: Modifier;
  setModifier: (m: Modifier) => void;
}) {
  const press = (k: Key) => {
    if (!term) return;
    if ("mod" in k) {
      setModifier(modifier === k.mod ? null : k.mod);
      return;
    }
    term.input("arrow" in k ? (term.appCursor() ? "\x1bO" : "\x1b[") + k.arrow : k.seq);
  };

  const row = (keys: Key[]) =>
    keys.map((k) => (
      <button
        key={k.id}
        type="button"
        tabIndex={-1}
        title={k.title ?? (typeof k.label === "string" ? undefined : k.id)}
        aria-label={typeof k.label === "string" ? undefined : k.id}
        aria-pressed={"mod" in k ? modifier === k.mod : undefined}
        className={"key" + ("mod" in k && modifier === k.mod ? " kb-on" : "") + (k.wide ? " kb-wide" : "")}
        disabled={!term}
        onPointerDown={(e) => e.preventDefault()}
        onMouseDown={(e) => e.preventDefault()}
        onClick={() => press(k)}
      >
        {k.label}
      </button>
    ));

  return (
    <div className="keybar" role="group" aria-label="Terminal keys">
      <div className="keybar-row">{row(ROW1)}</div>
      <div className="keybar-row keybar-row2">{row(ROW2)}</div>
    </div>
  );
}
