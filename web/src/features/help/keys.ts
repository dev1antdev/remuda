import { t } from "@/lib/i18n";
// Every key the page answers, in groups, for the sheet behind ?. The one list to read; each binding lives where it is
// handled (the Shell, the composer, the review's keys.ts, the palette, the wall).

export type KeyRow = { keys: string[]; what: string }; // a chord per key cap, "⌘ K"
export type KeyGroup = { title: string; rows: KeyRow[] };

export function keyGroups(mac: boolean): KeyGroup[] {
  const mod = mac ? "⌘" : "Ctrl";
  return [
    {
      title: t("Everywhere"),
      rows: [
        { keys: [`${mod} K`], what: t("the command palette: any agent, any command") },
        { keys: ["?"], what: t("this sheet") },
        { keys: ["Alt N"], what: t("the next agent that needs you") },
        { keys: ["Esc"], what: t("close a dialog, a menu, the file viewer") },
        { keys: ["←", "→"], what: t("a panel's edge once Tab lands on it; ⇧ by more, ↵ its usual width") },
      ],
    },
    {
      title: t("Chat"),
      rows: [
        { keys: ["↵"], what: "send" },
        { keys: ["⇧ ↵"], what: t("a new line") },
        { keys: ["↑", "↓"], what: t("what you sent before, on an empty line") },
        { keys: [`${mod} F`], what: t("find in this conversation") },
        { keys: ["↑", "↓", "↵", "Esc"], what: t("answer the agent's question with the buttons under it") },
      ],
    },
    {
      title: t("Changes"),
      rows: [
        { keys: ["j", "k"], what: t("the next or previous file") },
        { keys: ["↑", "↓"], what: t("the lines of the open file") },
        { keys: ["n", "p"], what: t("the next or previous hunk") },
        { keys: ["c"], what: t("comment on the line") },
        { keys: ["o"], what: t("open the file there") },
        { keys: ["s"], what: t("one column or two") },
        { keys: ["Esc"], what: t("leave the line") },
      ],
    },
    {
      title: t("Palette"),
      rows: [
        { keys: ["↑", "↓"], what: "move" },
        { keys: ["↵"], what: "run" },
        { keys: ["Esc"], what: "close" },
      ],
    },
    {
      title: t("Wall"),
      rows: [
        { keys: ["Esc"], what: t("back to the grid from a focused tile, or out of office mode") },
        { keys: ["←", "→"], what: t("office mode: feature the previous or next agent, pinned") },
      ],
    },
    {
      title: t("Tasks"),
      rows: [
        { keys: [`${mod} ↵`], what: t("post a comment") },
        { keys: ["↵"], what: t("open the card under the focus") },
        { keys: [`${mod} click`, "⇧ click"], what: t("select a task, or the range up to it; a long press on a phone — the bar under the list moves, labels, finishes or deletes them") },
        { keys: ["Esc"], what: t("clear the selection") },
      ],
    },
    {
      title: t("Terminal"),
      rows: [
        { keys: [mac ? "⌘ F" : "Ctrl ⇧ F"], what: t("find in the scrollback") },
        { keys: ["any key"], what: t("goes to the app in the pane; the tab strip's key bar types ⏎, Esc, Tab, the arrows, and holds Ctrl or Alt for the next key") },
      ],
    },
  ];
}
