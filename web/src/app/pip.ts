// The pop-out: one small window that stays above every other app (Chrome's Document Picture-in-Picture), with a
// piece of this page drawn into it through a portal. The window is this module's; the store says what is in it.

import { alsoVisible } from "@/lib/visibility";
import { actions, type Selection } from "./store";

type PipOpener = { requestWindow(options?: { width?: number; height?: number }): Promise<Window>; window: Window | null };

const pip = (window as Window & { documentPictureInPicture?: PipOpener }).documentPictureInPicture;

export const popOutSupported = !!pip;

const WIDTH = 440;
const HEIGHT = 640;

let current: Window | null = null;

// popOutWindow is the open window, for the portal to draw into.
export const popOutWindow = () => current;

// popOut opens the window (closing the one before: the browser allows one) and puts the pane in it. Called from
// the click itself, since the browser wants the person's gesture behind it.
export async function popOut(selection: Selection): Promise<void> {
  if (!pip) return;
  closePopOut();
  let win: Window;
  try {
    win = await pip.requestWindow({ width: WIDTH, height: HEIGHT });
  } catch (e) {
    actions.setPopOut(null);
    actions.notify(`Could not open the window: ${e instanceof Error ? e.message : String(e)}`, { error: true });
    return;
  }
  dress(win.document);
  const forget = alsoVisible(win.document); // its polls go on while the tab behind it is hidden
  win.addEventListener("pagehide", () => {
    forget();
    if (current === win) {
      current = null;
      actions.setPopOut(null);
    }
  });
  current = win;
  actions.setPopOut(selection);
}

// closePopOut shuts the window; the pagehide it fires clears the store.
export function closePopOut(): void {
  current?.close();
  current = null;
}

// dress gives the new document this page's stylesheets (a built page links one file, the dev server injects
// style tags) and the body class the pop-out rules hang on.
function dress(doc: Document) {
  doc.documentElement.lang = document.documentElement.lang;
  doc.documentElement.dataset.theme = document.documentElement.dataset.theme;
  for (const sheet of Array.from(document.styleSheets)) {
    if (sheet.href) {
      const link = doc.createElement("link");
      link.rel = "stylesheet";
      link.href = sheet.href;
      doc.head.appendChild(link);
      continue;
    }
    try {
      const style = doc.createElement("style");
      style.textContent = Array.from(sheet.cssRules)
        .map((rule) => rule.cssText)
        .join("\n");
      doc.head.appendChild(style);
    } catch {
      // a sheet from another origin keeps its rules to itself
    }
  }
  doc.body.className = "popout";
}
