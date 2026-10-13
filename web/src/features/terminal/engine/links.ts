// File paths and URLs on the screen become links: a click opens the file where it lives, at the :line if
// given, or the URL in a new tab. File links only in an agent's pane; in a shell's pane only a Cmd/Ctrl-click opens a URL.

import type { IDisposable, ILink, Terminal } from "@xterm/xterm";
import type { OpenFile } from "@/features/files/target";
import { openUrl } from "@/lib/host";
import { findPaths, findUrls } from "@/lib/paths";
import type { Flags } from "./pointer";

export function provideLinks(term: Terminal, props: () => { agent: boolean; onOpenFile: OpenFile }, flags: Flags): IDisposable {
  return term.registerLinkProvider({
    provideLinks(y, callback) {
      const row = term.buffer.active.getLine(y - 1);
      if (!row) return callback(undefined);
      const text = row.translateToString(true);
      const decorations = { pointerCursor: true, underline: true };
      const links: ILink[] = findUrls(text).map((m) => ({
        range: { start: { x: m.start + 1, y }, end: { x: m.end, y } },
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
            range: { start: { x: m.start + 1, y }, end: { x: m.end, y } },
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
