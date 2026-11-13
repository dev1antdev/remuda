// The Russian words of a Windows PC in the fleet: the screen terminal, and what such a machine has no editor for.

import type { Dictionary } from "../../i18n";

export const windows: Dictionary = {
  "Type into the pane": "Ввод в панель",
  "Why a screen and not a live terminal": "Почему экран, а не живой терминал",
  "The pane's screen, typed into: herdr has no live terminal on Windows": "Экран панели с вводом: у herdr на Windows нет живого терминала",
  "herdr on Windows has no terminal attach, so this is the pane's screen read a few times a second and typed into. For a live terminal, run herdr --remote {host} in a terminal on your Mac.":
    "У herdr на Windows нет подключения к терминалу, поэтому здесь экран панели: он перечитывается несколько раз в секунду, и в него можно печатать. Для живого терминала запустите herdr --remote {host} в терминале на вашем Mac.",
  "no code-server for Windows": "для Windows нет code-server",
  "No machine here runs code-server: there is none for Windows.": "Ни на одной машине здесь нет code-server: для Windows его нет.",
  "the terminal is the pane's screen, no Code mode": "терминал — это экран панели, режима Code нет",
};
