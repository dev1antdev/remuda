import type { ReactNode } from "react";
import { actions } from "@/app/store";
import Icon from "@/ui/icons";
import type { Modifier, TermApi } from "@/features/terminal/engine/types";
import { t } from "@/lib/i18n";

type Key = { id: string; label: ReactNode; title?: string; wide?: boolean } & ({ seq: string } | { arrow: "A" | "B" | "C" | "D" } | { mod: Exclude<Modifier, null> } | { find: true });

// ROW1 is what a phone keyboard lacks and an agent TUI needs constantly, ROW2 the rest of a
// keyboard; the sequences are what a terminal sends, arrows following DECCKM.
const ROW1: Key[] = [
  { id: t("Esc"), label: t("Esc"), seq: "\x1b" },
  { id: t("Tab"), label: t("Tab"), seq: "\t" },
  { id: t("Up"), label: <Icon name="chevron-up" size={14} />, arrow: "A" },
  { id: t("Down"), label: <Icon name="chevron-down" size={14} />, arrow: "B" },
  { id: t("Left"), label: <Icon name="chevron-left" size={14} />, arrow: "D" },
  { id: t("Right"), label: <Icon name="chevron-right" size={14} />, arrow: "C" },
  { id: t("Newline"), label: <Icon name="corner-down-left" size={14} />, seq: "\x1b\r", title: "Newline without sending (Alt+Enter)" },
  { id: t("Enter"), label: t("Enter"), seq: "\r", wide: true },
  { id: t("Ctrl"), label: t("Ctrl"), mod: "ctrl", title: t("Hold Ctrl for the next key") },
  { id: "^C", label: "^C", seq: "\x03", title: t("Interrupt") },
];

const ROW2: Key[] = [
  { id: t("Alt"), label: t("Alt"), mod: "alt", title: t("Hold Alt for the next key") },
  { id: "Shift+Tab", label: "⇤", seq: "\x1b[Z", title: "Shift+Tab" },
  { id: t("Home"), label: t("Home"), seq: "\x1b[H" },
  { id: t("End"), label: t("End"), seq: "\x1b[F" },
  { id: "PgUp", label: "PgUp", seq: "\x1b[5~" },
  { id: "PgDn", label: "PgDn", seq: "\x1b[6~" },
  { id: t("Backspace"), label: "⌫", seq: "\x7f", title: t("Backspace") },
  { id: t("Del"), label: t("Del"), seq: "\x1b[3~" },
  { id: "^D", label: "^D", seq: "\x04" },
  { id: "^Z", label: "^Z", seq: "\x1a" },
  { id: "^L", label: "^L", seq: "\x0c", title: t("Clear screen") },
  { id: "^U", label: "^U", seq: "\x15", title: t("Clear line") },
  { id: "^R", label: "^R", seq: "\x12", title: t("Search history") },
  { id: "^A", label: "^A", seq: "\x01" },
  { id: "^E", label: "^E", seq: "\x05" },
  { id: t("Find"), label: <Icon name="search" size={14} />, find: true, title: t("Find in the scrollback") },
];

// KeyBar is the terminal lens's key bar: every key goes through the live terminal's own input path, like
// a keystroke; pointer events are prevented so the terminal keeps focus and the soft keyboard stays up.
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
    if ("find" in k) {
      actions.openFind();
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
    <div className="keybar" role="group" aria-label={t("Terminal keys")}>
      <div className="keybar-row">{row(ROW1)}</div>
      <div className="keybar-row keybar-row2">{row(ROW2)}</div>
    </div>
  );
}
