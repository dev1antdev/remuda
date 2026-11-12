// A dev server a pane announced, opened as a preview from the corner's toast: the tab opens on the click itself, so
// no popup blocker takes it, and goes to the preview once the cockpit has it open; the server opens only a port it
// finds listening.

import { actions } from "@/app/store";
import { openPreview } from "@/features/preview/api";
import { errMsg } from "@/lib/text";

export async function previewServed(machineId: string, port: number): Promise<void> {
  const tab = window.open("about:blank", "_blank");
  if (tab) tab.opener = null;
  try {
    const preview = await openPreview(machineId, port);
    if (tab) tab.location.href = preview.url;
    else window.open(preview.url, "_blank", "noopener,noreferrer");
  } catch (e) {
    tab?.close();
    actions.notify(errMsg(e), { error: true });
  }
}
