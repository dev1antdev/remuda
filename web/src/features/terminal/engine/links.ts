// File paths and URLs on the screen become links: a click opens the file where it lives, at the :line if
// given, or the URL in a new tab. File links only in an agent's pane; in a shell's pane only a Cmd/Ctrl-click opens a URL.

import type { IBufferLine, IBufferRange, IDisposable, ILink, Terminal } from "@xterm/xterm";
import type { OpenFile } from "@/features/files/target";
import { openUrl } from "@/lib/host";
import { findPaths, findUrls } from "@/lib/paths";
import type { Flags } from "./pointer";

// RowCells is a row's text as the matchers read it, with the cell each UTF-16 unit of it sits on and that cell's width
type RowCells = { text: string; cell: number[]; width: number[] };

// rowCells reads a row cell by cell as xterm's translateToString does — a wide character's spacer skipped, an empty cell a
// space — so a match's offsets in the text become cells: a wide character fills two, a surrogate pair is one character
function rowCells(line: IBufferLine): RowCells {
  const row: RowCells = { text: "", cell: [], width: [] };
  for (let x = 0; x < line.length; x++) {
    const at = line.getCell(x);
    const width = at?.getWidth() ?? 1;
    if (width === 0) continue;
    const chars = at?.getChars() || " ";
    for (let unit = 0; unit < chars.length; unit++) {
      row.cell.push(x);
      row.width.push(width);
    }
    row.text += chars;
  }
  return row;
}

// cellRange is a match's place on the screen: 1-based cells, the end the last cell of its last character
function cellRange(row: RowCells, start: number, end: number, y: number): IBufferRange {
  return { start: { x: row.cell[start]! + 1, y }, end: { x: row.cell[end - 1]! + row.width[end - 1]!, y } };
}

export function provideLinks(term: Terminal, props: () => { agent: boolean; onOpenFile: OpenFile }, flags: Flags): IDisposable {
  return term.registerLinkProvider({
    provideLinks(y, callback) {
      const line = term.buffer.active.getLine(y - 1);
      if (!line) return callback(undefined);
      const row = rowCells(line);
      const text = row.text;
      const decorations = { pointerCursor: true, underline: true };
      const links: ILink[] = findUrls(text).map((m) => ({
        range: cellRange(row, m.start, m.end, y),
        text: m.text,
        decorations,
        activate: (e: MouseEvent) => {
          if (!props().agent && !e.metaKey && !e.ctrlKey) return;
          flags.linkUsed = true;
          openUrl(m.url);
        },
      }));
      if (props().agent) {
        for (const m of findPaths(text)) {
          links.push({
            range: cellRange(row, m.start, m.end, y),
            text: m.text,
            decorations,
            activate: () => {
              flags.linkUsed = true;
              props().onOpenFile({ path: m.path, line: m.line });
            },
          });
        }
      }
      callback(links.length ? links : undefined);
    },
  });
}
