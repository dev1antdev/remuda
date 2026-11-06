// The floating terminal's rules, without React: the key that opens and hides it, and the machine it opens on.

export type KeyPress = { code: string; ctrlKey: boolean; metaKey: boolean; altKey: boolean; shiftKey: boolean };

// isFloatKey is Ctrl+` by key code, whatever the layout and on a Mac too (⌘` switches the browser's windows there):
// it opens and hides the drawer from anywhere, the terminals included, where it would only type a NUL
export function isFloatKey(press: KeyPress): boolean {
  return press.code === "Backquote" && press.ctrlKey && !press.metaKey && !press.altKey && !press.shiftKey;
}

// floatKeyTakes says whether the key opens or hides the drawer now: not while a dialog, a panel such as the palette, the
// confirm, the file viewer or a menu is open over the page, since the drawer would open under it
export const floatKeyTakes = (press: KeyPress, overlayOpen: boolean): boolean => isFloatKey(press) && !overlayOpen;

export type ShellView = "opening" | "live" | "ended" | "away";

// shellView is what the drawer shows: its machine away (the roster then lists none of its panes, so the shell may well
// still run), the shell being opened, the shell, or the shell ended — gone from the roster after it was listed there
export function shellView(pane: string, listed: boolean, seen: boolean, connected: boolean): ShellView {
  if (!connected) return "away";
  if (!pane) return "opening";
  return seen && !listed ? "ended" : "live";
}

export type MachineState = { id: string; state: string };

// floatMachine is the machine the drawer opens on: the one asked while it is connected, else the pane's on screen, else
// this one, else the first connected; null when none is
export function floatMachine(asked: string | null, current: string | null, machines: MachineState[]): string | null {
  const connected = (id: string | null) => !!id && machines.some((m) => m.id === id && m.state === "connected");
  if (connected(asked)) return asked;
  if (connected(current)) return current;
  if (connected("local")) return "local";
  return machines.find((m) => m.state === "connected")?.id ?? null;
}

// keyWords is how the sheet and the tooltips write the key
export const keyWords = (mac: boolean) => (mac ? "⌃ `" : "Ctrl `");
