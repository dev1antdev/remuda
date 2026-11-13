// The files rail beside a pane's lenses, in two tabs: Agent, the files the agent touched this session, and Project, the
// workspace's folder as a tree with git's marks. The tab is a pref of this device; only the tab on screen reads anything.

import { useState } from "react";
import AgentFiles from "@/features/chat/AgentFiles";
import ProjectTree from "@/features/files/tree/ProjectTree";
import type { OpenFile } from "@/features/files/target";
import { usePref } from "@/lib/prefs";
import { basename } from "@/lib/text";
import { t } from "@/lib/i18n";

export type RailTab = "agent" | "project";

export default function FilesRail({ machineId, paneId, status, dir, active, viewOnly, editor, onOpenFile, onOpenInCode }: {
  machineId: string;
  paneId: string;
  status: string;
  dir: string;
  active: boolean;
  viewOnly: boolean;
  editor: boolean; // the machine has the Code mode's editor
  onOpenFile: OpenFile;
  onOpenInCode?: (path: string) => void;
}) {
  const [asked, setTab] = usePref<RailTab>("footprint.tab", "agent");
  const tab: RailTab = asked === "project" ? "project" : "agent";
  const [summary, setSummary] = useState("…");
  return (
    <aside className="footprint" aria-label={t("Files")}>
      <div className="footprint-head">
        <nav className="fv-tabs footprint-tabs" aria-label={t("Files")}>
          <button type="button" className={tab === "agent" ? "on" : ""} aria-pressed={tab === "agent"} onClick={() => setTab("agent")}>
            {t("Agent")}
          </button>
          <button type="button" className={tab === "project" ? "on" : ""} aria-pressed={tab === "project"} onClick={() => setTab("project")}>
            {t("Project")}
          </button>
        </nav>
        <span className="footprint-sub" title={tab === "project" ? dir : summary}>
          {tab === "project" ? basename(dir) : summary}
        </span>
      </div>
      {tab === "agent" ? (
        <AgentFiles machineId={machineId} paneId={paneId} status={status} dir={dir} active={active} onOpenFile={onOpenFile} onOpenInCode={onOpenInCode} onSummary={setSummary} />
      ) : (
        <ProjectTree machineId={machineId} paneId={paneId} dir={dir} active={active} viewOnly={viewOnly} editor={editor} onOpenFile={onOpenFile} />
      )}
    </aside>
  );
}
