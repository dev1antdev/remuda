// Export as GIF: the recording drawn frame by frame at the speed picked, saved as a file; while it draws, the button
// says how far it got and calls it off.

import { useEffect, useRef, useState } from "react";
import { actions } from "@/app/store";
import { saveBlob } from "@/lib/host";
import { t } from "@/lib/i18n";
import { errMsg } from "@/lib/text";
import type { AnsiPalette } from "@/lib/theme";
import { Icon } from "@/ui";
import type { Recording } from "./api";
import type { ScreenRows, Speed } from "./playback";
import { exportGif } from "./raster";
import { gifName } from "./words";

export function GifExport({ recording, screens, times, speed, palette }: { recording: Recording; screens: ScreenRows[]; times: number[]; speed: Speed; palette: AnsiPalette }) {
  const [progress, setProgress] = useState<number | null>(null); // 0..1 while it draws
  const running = useRef<AbortController | null>(null);
  useEffect(() => () => running.current?.abort(), []);

  const start = async () => {
    const control = new AbortController();
    running.current = control;
    setProgress(0);
    try {
      const font = getComputedStyle(document.documentElement).getPropertyValue("--mono").trim() || "monospace";
      const gif = await exportGif({ screens, times, cols: recording.cols }, palette, font, speed, (done, total) => setProgress(done / total), control.signal);
      saveBlob(gifName(recording, speed), gif);
      actions.notify(t("Saved the timelapse as a GIF"), { brief: true });
    } catch (e) {
      if (!control.signal.aborted) actions.notify(errMsg(e), { error: true });
    } finally {
      if (running.current === control) running.current = null;
      setProgress(null);
    }
  };
  if (progress !== null)
    return (
      <button type="button" className="btn sm timelapse-export" onClick={() => running.current?.abort()}>
        <Icon name="x" size={14} /> {t("Drawing the GIF… {pct} %", { pct: Math.round(progress * 100) })}
      </button>
    );
  return (
    <button type="button" className="btn sm timelapse-export" disabled={screens.length === 0} onClick={() => void start()}>
      <Icon name="download" size={14} /> {t("Export as GIF")}
    </button>
  );
}
