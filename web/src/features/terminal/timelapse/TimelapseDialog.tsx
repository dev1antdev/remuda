// Timelapse: what the pane's screen did while its agent worked, recorded on the cockpit every few seconds and played
// back in seconds. The recordings of this pane come first, then those of the earlier panes that worked in its folder;
// under the screen-sharing mode a hidden folder's are not shown. Delete is the person's; a viewer link only plays.

import { useEffect, useState } from "react";
import { actions } from "@/app/store";
import { useRoster } from "@/features/roster/useRoster";
import { useSharing } from "@/features/roster/useSharing";
import { t } from "@/lib/i18n";
import { useQuery } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { Dialog, Icon, Notice, Select } from "@/ui";
import { deleteRecording, fetchRecording, listRecordings } from "./api";
import { Player } from "./Player";
import { recordingLabel, shownRecordings } from "./words";

export default function TimelapseDialog({ machineId, paneId, dir, title, onClose }: { machineId: string; paneId: string; dir: string; title: string; onClose: () => void }) {
  const { viewOnly } = useRoster();
  const share = useSharing();
  const { data: listed, error, reload } = useQuery(() => listRecordings(machineId, paneId, dir), [machineId, paneId, dir]);
  const recordings = shownRecordings(listed?.recordings ?? [], { machine: share.machineHidden, folder: share.folderHidden });
  const hiddenAll = (listed?.recordings.length ?? 0) > 0 && recordings.length === 0;
  const [picked, setPicked] = useState("");
  const chosen = recordings.find((r) => r.id === picked) ?? recordings.find((r) => r.pane === paneId) ?? recordings[0];
  useEffect(() => {
    if (chosen && chosen.id !== picked) setPicked(chosen.id);
  }, [chosen, picked]);
  const { data: recording, error: playError } = useQuery(() => (chosen ? fetchRecording(chosen.id) : Promise.resolve(null)), [chosen?.id], { enabled: !!chosen });

  const remove = () => {
    if (!chosen) return;
    actions.ask(t("Delete this recording? The turn's conversation stays as it is."), t("Delete"), () =>
      void deleteRecording(chosen.id)
        .then(() => {
          setPicked("");
          reload();
        })
        .catch((e) => actions.notify(errMsg(e), { error: true })),
    );
  };
  return (
    <Dialog title={t("Timelapse")} icon="film" size="lg" className="timelapse-dialog" onClose={onClose}>
      <h3 className="standup-title">{share.redact(title)}</h3>
      {error && <Notice tone="error">{error}</Notice>}
      {listed && recordings.length === 0 && (
        <Notice tone="hint">
          {hiddenAll
            ? t("This folder is hidden while you share the screen.")
            : listed.on
              ? t("No recording of this pane or its folder yet. A turn is recorded while the agent works, and kept once it has worked a minute or more.")
              : t("Recording is off: turn it on in Settings › Timelapse.")}
        </Notice>
      )}
      {recordings.length > 0 && chosen && (
        <>
          <div className="timelapse-pick">
            <Select value={chosen.id} onChange={setPicked} options={recordings.map((r) => ({ value: r.id, label: recordingLabel(r, paneId) }))} />
            {!viewOnly && (
              <button type="button" className="btn sm danger" onClick={remove}>
                <Icon name="trash" size={14} /> {t("Delete")}
              </button>
            )}
          </div>
          {playError && <Notice tone="error">{playError}</Notice>}
          {recording && recording.id === chosen.id ? <Player recording={recording} /> : !playError && <div className="empty chat-empty">{t("Loading…")}</div>}
        </>
      )}
    </Dialog>
  );
}
