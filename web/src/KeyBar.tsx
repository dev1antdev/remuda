import type { Modifier, TermApi } from "./Terminal";

type Key =
  | { label: string; seq: string; title?: string; wide?: boolean }
  | { label: string; arrow: "A" | "B" | "C" | "D"; title?: string }
  | { label: string; mod: Exclude<Modifier, null>; title?: string };

// Row 1: what a phone keyboard lacks and an agent TUI needs constantly.
// Row 2: the rest of a keyboard. Sequences are what a terminal would send;
// arrows follow DECCKM, so they match whatever the TUI asked for.
const ROW1: Key[] = [
  { label: "Esc", seq: "\x1b" },
  { label: "Tab", seq: "\t" },
  { label: "↑", arrow: "A" },
  { label: "↓", arrow: "B" },
  { label: "←", arrow: "D" },
  { label: "→", arrow: "C" },
  { label: "⏎", seq: "\x1b\r", title: "Newline without sending (Alt+Enter)" },
  { label: "Enter", seq: "\r", wide: true },
  { label: "Ctrl", mod: "ctrl", title: "Hold Ctrl for the next key" },
  { label: "^C", seq: "\x03", title: "Interrupt" },
];

const ROW2: Key[] = [
  { label: "Alt", mod: "alt", title: "Hold Alt for the next key" },
  { label: "⇤", seq: "\x1b[Z", title: "Shift+Tab" },
  { label: "Home", seq: "\x1b[H" },
  { label: "End", seq: "\x1b[F" },
  { label: "PgUp", seq: "\x1b[5~" },
  { label: "PgDn", seq: "\x1b[6~" },
  { label: "⌫", seq: "\x7f", title: "Backspace" },
  { label: "Del", seq: "\x1b[3~" },
  { label: "^D", seq: "\x04" },
  { label: "^Z", seq: "\x1a" },
  { label: "^L", seq: "\x0c", title: "Clear screen" },
  { label: "^U", seq: "\x15", title: "Clear line" },
  { label: "^R", seq: "\x12", title: "Search history" },
  { label: "^A", seq: "\x01" },
  { label: "^E", seq: "\x05" },
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
        key={k.label}
        type="button"
        tabIndex={-1}
        title={k.title}
        className={
          ("mod" in k && modifier === k.mod ? "kb-on " : "") + ("wide" in k && k.wide ? "kb-wide" : "")
        }
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
