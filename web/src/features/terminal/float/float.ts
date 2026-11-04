// The floating terminal's rules, without React: the key that opens and hides it, and the machine it opens on.

export type KeyPress = { code: string; ctrlKey: boolean; metaKey: boolean; altKey: boolean; shiftKey: boolean };

// isFloatKey is Ctrl+` by key code, whatever the layout and on a Mac too (⌘` switches the browser's windows there):
// it opens and hides the drawer from anywhere, the terminals included, where it would only type a NUL
export function isFloatKey(press: KeyPress): boolean {
  return press.code === "Backquote" && press.ctrlKey && !press.metaKey && !press.altKey && !press.shiftKey;
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
