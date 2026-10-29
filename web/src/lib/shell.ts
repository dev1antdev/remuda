// The native shell's bridge. The Mac app announces itself as window.remudaShell before the page loads; the page tells
// it what to show in the menu bar and on the Dock through the app's message handler, and hangs the functions the app
// calls on that same object. Without the app nothing here does anything: inShell() is false and tellShell() is a no-op.

export type ShellTone = "red" | "amber" | "green";

// ShellRow is one agent that needs the person, as the menu bar lists it
export type ShellRow = {
  machine: string;
  machineLabel: string;
  pane: string;
  workspace: string;
  title: string;
  said: string;
  tone: ShellTone;
  state: string;
};

export type ShellMessage =
  | { type: "ready"; version: string; machine: string }
  | { type: "badge"; count: number; tone: ShellTone | null }
  | { type: "menu"; rows: ShellRow[]; working: number; idle: number };

// ShellHandlers are the functions the page defines for the app to call
export type ShellHandlers = {
  select?: (machine: string, pane: string) => void;
  openPanel?: (name: string) => void;
  openMode?: (mode: string) => void;
  alert?: (payload: unknown) => void;
};

export type RemudaShell = { native?: boolean; version?: string } & ShellHandlers;

declare global {
  interface Window {
    remudaShell?: RemudaShell;
    webkit?: { messageHandlers?: { remuda?: { postMessage: (message: unknown) => void } } };
  }
}

// inShell says whether the page runs inside the Mac app
export function inShell(): boolean {
  return typeof window !== "undefined" && window.remudaShell?.native === true;
}

// tellShell hands one message to the app; true when it was taken, false when there is no app to take it
export function tellShell(message: ShellMessage): boolean {
  if (typeof window === "undefined") return false;
  try {
    const handler = window.webkit?.messageHandlers?.remuda;
    if (!handler) return false;
    handler.postMessage(message);
    return true;
  } catch {
    return false;
  }
}

// handShell puts the page's functions on the app's object, keeping what the app put there
export function handShell(handlers: ShellHandlers): void {
  if (typeof window === "undefined" || !window.remudaShell) return;
  Object.assign(window.remudaShell, handlers);
}
