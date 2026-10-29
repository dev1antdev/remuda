// Waking a sleeping machine: the menu rows, the address dialog and the toasts.

import type { Dictionary } from "../../i18n";

export const wake: Dictionary = {
  "Wake up": "Разбудить",
  "MAC address…": "MAC-адрес…",
  "MAC address": "MAC-адрес",
  "MAC address of {machine}": "MAC-адрес {machine}",
  "Keep": "Сохранить",
  "An address looks like aa:bb:cc:dd:ee:ff": "Адрес выглядит как aa:bb:cc:dd:ee:ff",
  "Wake up sends the magic packet to this address on every network this remuda is on — the machine must be on the same one, with Wake-on-LAN allowed in its firmware. Leave it empty to forget the address.":
    "«Разбудить» шлёт magic packet на этот адрес во все сети, где есть этот remuda; машина должна быть в той же сети, а Wake-on-LAN разрешён в её прошивке. Оставьте пустым, чтобы забыть адрес.",
  "Woke {machine} — reconnecting": "Разбудили {machine}, переподключаемся",
  "Nothing could carry the packet to {machine}": "Пакет для {machine} отправить было некуда",
  "Rename, wake, copy the SSH host, remove": "Переименовать, разбудить, скопировать SSH-хост, удалить",
  "{machine}: rename, wake, copy the SSH host, remove": "{machine}: переименовать, разбудить, скопировать SSH-хост, удалить",
};
