// Files dropped or pasted on the terminal are uploaded into the pane's folder and their paths typed
// shell-quoted, never sent; a multi-line paste is bracketed by hand when nobody knows whether the app asked.

import type { Terminal } from "@xterm/xterm";
import type { Upload } from "@/features/files/api";
import { shellQuote } from "@/lib/text";
import { filesFrom } from "@/lib/transfer";
import { bracketed } from "./keys";

export function installPaste(term: Terminal, mount: HTMLElement, { upload, sendInput, pasteModeSeen }: { upload: (files: File[]) => Promise<Upload[]>; sendInput: (data: string) => void; pasteModeSeen: () => boolean }): () => void {
  // uploads are serialized so two drops don't interleave their paths
  let uploads: Promise<void> = Promise.resolve();
  const typeFiles = (files: File[]) => {
    uploads = uploads.then(async () => {
      try {
        const ups = await upload(files);
        sendInput(ups.map((u) => shellQuote(u.path)).join(" ") + " ");
      } catch (e) {
        console.warn("upload failed:", e);
      }
    });
  };
  const onDragOver = (e: DragEvent) => {
    if (e.dataTransfer?.types.includes("Files")) {
      e.preventDefault();
      mount.classList.add("drag");
    }
  };
  const onDragLeave = () => mount.classList.remove("drag");
  const onDrop = (e: DragEvent) => {
    mount.classList.remove("drag");
    const files = filesFrom(e.dataTransfer);
    if (files.length === 0) return;
    e.preventDefault();
    typeFiles(files);
  };
  const onPaste = (e: ClipboardEvent) => {
    const files = filesFrom(e.clipboardData);
    if (files.length > 0) {
      e.preventDefault();
      e.stopPropagation();
      typeFiles(files);
      return;
    }
    // a late attach never saw DECSET 2004 and xterm would paste a multi-line text raw, every newline an
    // Enter that sends the prompt: it is bracketed by hand instead (CRLF/LF → CR, as xterm does)
    const text = e.clipboardData?.getData("text/plain") ?? "";
    if (!text.includes("\n") || term.modes.bracketedPasteMode || pasteModeSeen()) return;
    e.preventDefault();
    e.stopPropagation();
    sendInput(bracketed(text));
  };
  mount.addEventListener("dragover", onDragOver);
  mount.addEventListener("dragleave", onDragLeave);
  mount.addEventListener("drop", onDrop);
  mount.addEventListener("paste", onPaste, true);
  return () => {
    mount.removeEventListener("dragover", onDragOver);
    mount.removeEventListener("dragleave", onDragLeave);
    mount.removeEventListener("drop", onDrop);
    mount.removeEventListener("paste", onPaste, true);
  };
}
