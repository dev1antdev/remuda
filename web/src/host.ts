// Host integration: when herdr-ui runs inside something that owns an editor
// (the JetBrains plugin's embedded browser), the host registers itself on
// `window.herdrHost` and gets to open files itself; herdr-ui in turn exposes
// `window.herdrUI` so the host can push text into the composer.

import type { FileTarget } from "./FileViewer";

export type HerdrHost = {
  // open a file in the host's editor instead of the web viewer; return false
  // to fall back to the viewer
  openFile?: (path: string, line: number | null, machineId: string) => boolean;
  // put text on the user's clipboard — an embedded browser in Remote
  // Development writes to the wrong machine's clipboard otherwise
  copy?: (text: string) => boolean;
  // open a web link in the user's browser — an embedded browser has no
  // tabs, and window.open there would navigate herdr-ui away
  openUrl?: (url: string) => boolean;
};

export type HerdrUI = {
  // insert text into the chat composer at the caret (e.g. "@src/app.php")
  insert: (text: string) => void;
  // select the pane whose folder is this path, when the roster has one
  selectFolder: (cwd: string) => void;
};

declare global {
  interface Window {
    herdrHost?: HerdrHost;
    herdrUI?: HerdrUI;
  }
}

export const INSERT_EVENT = "herdr-ui:insert";
export const SELECT_FOLDER_EVENT = "herdr-ui:select-folder";

// hostOpensFile asks the host to open the file; true when it did.
export function hostOpensFile(machineId: string, t: FileTarget): boolean {
  const fn = window.herdrHost?.openFile;
  if (!fn) return false;
  try {
    return fn(t.path, t.line ?? null, machineId) === true;
  } catch {
    return false;
  }
}

// copyText puts text on the clipboard: through the host when it offers to,
// else the Clipboard API, else the old execCommand path.
export async function copyText(text: string): Promise<boolean> {
  try {
    if (window.herdrHost?.copy?.(text) === true) return true;
  } catch {
    /* fall through */
  }
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    const ta = document.createElement("textarea");
    ta.value = text;
    ta.style.position = "fixed";
    ta.style.opacity = "0";
    document.body.appendChild(ta);
    ta.select();
    const ok = document.execCommand("copy");
    ta.remove();
    return ok;
  }
}

// openUrl opens a link in a new tab: the host's browser when it offers one,
// else this browser's.
export function openUrl(url: string): void {
  try {
    if (window.herdrHost?.openUrl?.(url) === true) return;
  } catch {
    /* fall through */
  }
  window.open(url, "_blank", "noopener,noreferrer");
}

export function installHostAPI(): void {
  window.herdrUI = {
    insert: (text) => window.dispatchEvent(new CustomEvent(INSERT_EVENT, { detail: text })),
    selectFolder: (cwd) => window.dispatchEvent(new CustomEvent(SELECT_FOLDER_EVENT, { detail: cwd })),
  };
}
