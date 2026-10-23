// The lines a recap reads as on the page: the numbers, the calls, the files, the last answer.

import type { Recap } from "./api";
import { calls, fileList, numbers } from "./standup";

export function RecapBody({ recap, dir }: { recap: Recap; dir: string }) {
  return (
    <>
      <div className="standup-line">{numbers(recap)}</div>
      {recap.calls > 0 && <div className="standup-line quiet">{calls(recap)}</div>}
      {recap.edited.length > 0 && (
        <div className="standup-files" title={recap.edited.join("\n")}>
          <span className="standup-label">Edited</span>
          {fileList(recap.edited, dir)}
        </div>
      )}
      {recap.read.length > 0 && (
        <div className="standup-files" title={recap.read.join("\n")}>
          <span className="standup-label">Read</span>
          {fileList(recap.read, dir)}
        </div>
      )}
      {recap.commands.length > 0 && (
        <div className="standup-files" title={recap.commands.join("\n")}>
          <span className="standup-label">Ran</span>
          {recap.commands.slice(0, 4).join(" · ")}
          {recap.commands.length > 4 ? ` · +${recap.commands.length - 4} more` : ""}
        </div>
      )}
      {recap.asking && <div className="standup-line is-blocked">A question is open.</div>}
      {recap.said && (
        <div className="standup-said" title={recap.said}>
          {recap.said}
        </div>
      )}
    </>
  );
}
