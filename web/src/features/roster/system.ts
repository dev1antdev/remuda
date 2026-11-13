// What a machine's system means for the page: a Windows PC's panes have no live terminal (herdr has no terminal
// attach there, so the lens is the screen terminal) and no code-server, so no Code mode and no "Open in the editor". Pure.

import { t } from "@/lib/i18n";
import type { Option } from "@/ui/Select";

type Located = { os?: string; host?: string } | null | undefined;

// onWindows says the machine is a Windows PC, by the roster's os
export const onWindows = (machine: Located): boolean => machine?.os === "windows";

// systemWords names the system as a person reads it; "" while the roster does not know it
export function systemWords(os: string | undefined): string {
  switch (os) {
    case "windows":
      return "Windows";
    case "darwin":
      return "macOS";
    case "linux":
      return "Linux";
    default:
      return "";
  }
}

// noEditorWords is why a Windows machine is not offered as the Code mode's pick
export const noEditorWords = () => t("no code-server for Windows");

// editorOption is a machine as the Code mode's pick lists it: its ssh host beside it, or, for a Windows PC, the reason
// it cannot be picked
export function editorOption(option: Option<string>, machine: Located): Option<string> {
  return onWindows(machine) ? { ...option, hint: noEditorWords(), disabled: true } : { ...option, hint: machine?.host };
}

// screenTerminalWords is why a Windows machine's terminal is a screen read again and again, and where a live one is
export const screenTerminalWords = (host: string) =>
  t("herdr on Windows has no terminal attach, so this is the pane's screen read a few times a second and typed into. For a live terminal, run herdr --remote {host} in a terminal on your Mac.", { host: host || "<pc>" });
