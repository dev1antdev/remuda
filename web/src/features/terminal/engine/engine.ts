// The terminal engine: one xterm on one herdr pane, outside React. Pane bytes arrive over the transport
// and are painted; keystrokes go back; herdr keeps the real scrollback. The pieces — the modes intercepted,
// the mouse reports, the selection that stays with the text, the selection layer, copy, links, paste — are
// wired here and torn down together.

import { FitAddon } from "@xterm/addon-fit";
import { Terminal as XTerm } from "@xterm/xterm";
import "@xterm/xterm/css/xterm.css";
import { uploadFiles } from "@/features/files/api";
import type { OpenFile } from "@/features/files/target";
import { installCopy } from "./copy";
import { applyGeometry, keepFollower, type Geometry } from "./geometry";
import { applyModifier } from "./keys";
import { createLayer } from "./layer";
import { provideLinks } from "./links";
import { interceptModes, splitTracking } from "./modes";
import { createReporter } from "./mouse";
import { installPaste } from "./paste";
import { installPointer, type Flags } from "./pointer";
import { trackSelection } from "./selection";
import { connect, type Transport } from "./transport";
import type { Modifier, TermApi } from "./types";
import { terminalTheme, type Flavor } from "@/lib/theme";

export type EngineProps = Geometry & {
  flavor: Flavor; // the page's flavour: the terminal's colours follow it
  agent: boolean; // an agent's pane: file paths and URLs are links; in a shell's pane only a Cmd/Ctrl-click opens a URL
  modifier: Modifier;
  onModifierUsed: () => void;
  onReady: (api: TermApi | null) => void;
  onOpenFile: OpenFile;
  onCopied: (chars: number) => void;
};

export type Engine = {
  apply: () => void; // the geometry props changed, or the mount came back on screen: measure again
  focus: () => void;
  select: (on: boolean) => void; // the selection layer
  dispose: () => void;
};

export function createEngine(mount: HTMLElement, machineId: string, paneId: string, props: () => EngineProps): Engine {
  const term = new XTerm({
    fontFamily: "ui-monospace, SFMono-Regular, Menlo, monospace",
    fontSize: props().fontSize,
    scrollback: 2000,
    allowProposedApi: true,
    theme: terminalTheme(props().flavor),
  });
  let flavorNow = props().flavor;
  const fit = new FitAddon();
  term.loadAddon(fit);
  term.open(mount);

  let lastSize = "";
  let transport: Transport | null = null;
  const note = (text: string) => term.write(`\r\n\x1b[2m[remuda] ${text}\x1b[0m\r\n`);
  // sendResize sends the grid to the shared PTY only when it changed since the connect request carried it.
  const sendResize = () => {
    const size = `${term.cols}x${term.rows}`;
    if (size === lastSize) return;
    lastSize = size;
    transport?.resize(term.cols, term.rows);
  };
  const sendInput = (data: string) => transport?.input(data);
  const input = (seq: string) => {
    const m = props().modifier;
    const out = applyModifier(m, seq);
    sendInput(out);
    if (m && out !== seq) props().onModifierUsed();
  };
  const apply = () => {
    if (props().flavor !== flavorNow) {
      flavorNow = props().flavor;
      term.options.theme = terminalTheme(flavorNow);
    }
    if (applyGeometry(term, fit, mount, props())) sendResize();
  };
  apply();

  const modes = interceptModes(term, mount);
  const reporter = createReporter(term, modes, sendInput);
  const selection = trackSelection(term, reporter, modes);
  const layer = createLayer(term);
  const flags: Flags = { linkUsed: false };
  const follow = keepFollower(mount);

  // paint writes a chunk from either transport; one streaming decoder, so a character cut by a chunk boundary joins.
  const decoder = new TextDecoder();
  const paint = (buf: Uint8Array) => {
    const text = decoder.decode(buf, { stream: true });
    term.write(splitTracking(text), () => {
      follow();
      layer.schedule();
      selection.onPainted();
    });
  };
  // onOpen resets the screen for a fresh stream: there is no replay, the server nudges the app into a
  // repaint, so the modes xterm knew are re-applied by hand since nobody will set them again.
  const onOpen = (sentSize: string) => {
    const appCursor = term.modes.applicationCursorKeysMode;
    const bracketedPaste = term.modes.bracketedPasteMode;
    decoder.decode(); // a character cut by the old stream's end is dropped
    term.reset();
    selection.forget();
    modes.forgetPaste();
    if (appCursor) term.write("\x1b[?1h");
    if (bracketedPaste) term.write("\x1b[?2004h");
    lastSize = sentSize;
    sendResize();
  };

  const onData = term.onData((data) => {
    selection.forget();
    input(data);
  });
  const uninstallPointer = installPointer(term, mount, { modes, reporter, selection, layer: layer.el, flags });
  const uninstallCopy = installCopy(term, mount, { layer: layer.el, selection, sendInput, onCopied: (n) => props().onCopied(n) });
  const links = provideLinks(term, props, flags);
  const uninstallPaste = installPaste(term, mount, { upload: (files) => uploadFiles(machineId, paneId, files), sendInput, pasteModeSeen: modes.pasteModeSeen });

  // debounced: a window-edge drag would flood the agent with resizes, and Claude Code repaints its whole conversation per resize
  let settle: ReturnType<typeof setTimeout> | null = null;
  const resizes = new ResizeObserver(() => {
    if (settle) clearTimeout(settle);
    settle = setTimeout(apply, 120);
  });
  resizes.observe(mount);

  transport = connect({
    query: `machine=${encodeURIComponent(machineId)}&id=${encodeURIComponent(paneId)}`,
    size: () => ({ cols: term.cols, rows: term.rows }),
    onOpen,
    onBytes: paint,
    note,
  });
  props().onReady({ input, appCursor: () => term.modes.applicationCursorKeysMode });

  return {
    apply,
    focus: () => term.focus(),
    select: (on) => (on ? layer.show() : layer.hide()),
    dispose: () => {
      props().onReady(null);
      onData.dispose();
      selection.dispose();
      layer.dispose();
      modes.dispose();
      uninstallPointer();
      uninstallCopy();
      links.dispose();
      uninstallPaste();
      resizes.disconnect();
      if (settle) clearTimeout(settle);
      transport?.close();
      transport = null;
      term.dispose();
    },
  };
}
