// One conversation's recap: the pane's live session or the one picked in the lens, or a transcript open from the search.

import { actions } from "@/app/store";
import { copyText } from "@/lib/host";
import { useQuery } from "@/lib/query";
import { Dialog, Icon, Notice } from "@/ui";
import { fetchRecap } from "./api";
import { RecapBody } from "./RecapBody";
import { recapMarkdown } from "./standup";
import { t } from "@/lib/i18n";

export default function RecapDialog({ machineId, paneId, session, dir, title, onClose }: { machineId: string; paneId: string; session: string | null; dir: string; title: string; onClose: () => void }) {
  const { data, error, loading } = useQuery(() => fetchRecap(machineId, paneId, session, dir), [machineId, paneId, session, dir]);
  const copy = async () => {
    if (!data) return;
    if (await copyText(recapMarkdown(title, data.recap, dir))) actions.notify(t("Copied the recap as Markdown"), { brief: true });
    else actions.notify(t("Could not copy"), { error: true });
  };
  // a stretch of the strip opens the conversation at that turn: the pane's chat, or the viewer for a transcript on its own
  const jump = (turn: string) => {
    if (paneId) actions.selectPane(machineId, paneId);
    actions.jumpTo(machineId, paneId, session ?? "", turn);
    onClose();
  };
  return (
    <Dialog title={t("Recap")} icon="sun" size="md" className="standup" onClose={onClose} controls={loading ? <span className="search-busy" /> : undefined}>
      <h3 className="standup-title">{title}</h3>
      {error && <Notice tone="error">{error}</Notice>}
      {data && (data.recap.prompts || data.recap.answers || data.recap.calls ? <RecapBody recap={data.recap} dir={dir} since={data.since || undefined} onJump={jump} /> : <Notice tone="hint">{t("Nothing in this conversation yet.")}</Notice>)}
      <div className="modal-actions">
        <button type="button" className="btn primary" disabled={!data} onClick={() => void copy()}>
          <Icon name="copy" size={14} /> {t("Copy as Markdown")}
        </button>
      </div>
    </Dialog>
  );
}
