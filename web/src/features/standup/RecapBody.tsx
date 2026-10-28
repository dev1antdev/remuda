// The lines a recap reads as on the page: the day strip, the numbers, the calls, the files, the last answer.

import type { Recap } from "./api";
import { calls, fileList, numbers } from "./standup";
import { DayStrip } from "./DayStrip";
import { t } from "@/lib/i18n";

export function RecapBody({ recap, dir, since, onJump }: { recap: Recap; dir: string; since?: string; onJump?: (turn: string) => void }) {
  return (
    <>
      {recap.spans && recap.spans.length > 0 && <DayStrip spans={recap.spans} since={since} onJump={onJump} />}
      <div className="standup-line">{numbers(recap)}</div>
      {recap.calls > 0 && <div className="standup-line quiet">{calls(recap)}</div>}
      {recap.edited.length > 0 && (
        <div className="standup-files" title={recap.edited.join("\n")}>
          <span className="standup-label">{t("Edited")}</span>
          {fileList(recap.edited, dir)}
        </div>
      )}
      {recap.read.length > 0 && (
        <div className="standup-files" title={recap.read.join("\n")}>
          <span className="standup-label">{t("Read")}</span>
          {fileList(recap.read, dir)}
        </div>
      )}
      {recap.commands.length > 0 && (
        <div className="standup-files" title={recap.commands.join("\n")}>
          <span className="standup-label">{t("Ran")}</span>
          {recap.commands.slice(0, 4).join(" · ")}
          {recap.commands.length > 4 ? ` · ${t("+{n} more", { n: recap.commands.length - 4 })}` : ""}
        </div>
      )}
      {recap.asking && <div className="standup-line is-blocked">{t("A question is open.")}</div>}
      {recap.said && (
        <div className="standup-said" title={recap.said}>
          {recap.said}
        </div>
      )}
    </>
  );
}
