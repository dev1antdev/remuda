// Every key the page answers, in groups, for the sheet behind ?. The one list to read; each binding lives where it is
// handled (the Shell, the composer, the review's keys.ts, the palette, the wall).

export type KeyRow = { keys: string[]; what: string }; // a chord per key cap, "⌘ K"
export type KeyGroup = { title: string; rows: KeyRow[] };

export function keyGroups(mac: boolean): KeyGroup[] {
  const mod = mac ? "⌘" : "Ctrl";
  return [
    {
      title: "Everywhere",
      rows: [
        { keys: [`${mod} K`], what: "the command palette: any agent, any command" },
        { keys: ["?"], what: "this sheet" },
        { keys: ["Alt N"], what: "the next agent that needs you" },
        { keys: ["Esc"], what: "close a dialog, a menu, the file viewer" },
        { keys: ["←", "→"], what: "a panel's edge once Tab lands on it; ⇧ by more, ↵ its usual width" },
      ],
    },
    {
      title: "Chat",
      rows: [
        { keys: ["↵"], what: "send" },
        { keys: ["⇧ ↵"], what: "a new line" },
        { keys: ["↑", "↓"], what: "what you sent before, on an empty line" },
        { keys: [`${mod} F`], what: "find in this conversation" },
        { keys: ["↑", "↓", "↵", "Esc"], what: "answer the agent's question with the buttons under it" },
      ],
    },
    {
      title: "Changes",
      rows: [
        { keys: ["j", "k"], what: "the next or previous file" },
        { keys: ["↑", "↓"], what: "the lines of the open file" },
        { keys: ["n", "p"], what: "the next or previous hunk" },
        { keys: ["c"], what: "comment on the line" },
        { keys: ["o"], what: "open the file there" },
        { keys: ["s"], what: "one column or two" },
        { keys: ["Esc"], what: "leave the line" },
      ],
    },
    {
      title: "Palette",
      rows: [
        { keys: ["↑", "↓"], what: "move" },
        { keys: ["↵"], what: "run" },
        { keys: ["Esc"], what: "close" },
      ],
    },
    {
      title: "Wall",
      rows: [
        { keys: ["Esc"], what: "back to the grid from a focused tile, or out of office mode" },
        { keys: ["←", "→"], what: "office mode: feature the previous or next agent, pinned" },
      ],
    },
    {
      title: "Tasks",
      rows: [
        { keys: [`${mod} ↵`], what: "post a comment" },
        { keys: ["↵"], what: "open the card under the focus" },
      ],
    },
    {
      title: "Terminal",
      rows: [{ keys: ["any key"], what: "goes to the app in the pane; the tab strip's key bar types ⏎, Esc, Tab, the arrows, and holds Ctrl or Alt for the next key" }],
    },
  ];
}
