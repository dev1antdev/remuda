// What a machine's processes serve — a dev server an agent started, a python3 -m http.server — the workspace's first
// and the machine's others after, each with Open: the port opened on the cockpit for this browser alone, in a new tab.

import { useState } from "react";
import { useQuery } from "@/lib/query";
import { errMsg, plural } from "@/lib/text";
import { Dialog, Icon, Notice } from "@/ui";
import { t } from "@/lib/i18n";
import { closePreview, fetchPreviews, openPreview, type Listener, type Preview } from "./api";
import { hostOf, listenerWords, openOn, split, whereWords } from "./previews";

export default function PreviewsDialog({ machineId, machineLabel, workspace, workspaceName, onClose }: {
  machineId: string;
  machineLabel: string;
  workspace?: string; // the workspace whose ports come first
  workspaceName?: string;
  onClose: () => void;
}) {
  const { data, error, loading, reload } = useQuery(() => fetchPreviews(machineId), [machineId], { every: 5000 });
  const [busy, setBusy] = useState(0);
  const [failure, setFailure] = useState("");
  const [showOthers, setShowOthers] = useState(false);
  const listeners = data?.listeners ?? [];
  const open = data?.open ?? [];
  const { here, workspaces, others } = split(listeners, workspace);

  // the tab opens on the click itself, so no popup blocker takes it, and goes to the preview once it is open
  const show = async (port: number) => {
    setFailure("");
    const tab = window.open("about:blank", "_blank");
    if (tab) tab.opener = null;
    setBusy(port);
    try {
      const preview = await openPreview(machineId, port);
      if (tab) tab.location.href = preview.url;
      else window.open(preview.url, "_blank", "noopener,noreferrer");
      reload();
    } catch (e) {
      tab?.close();
      setFailure(errMsg(e));
    } finally {
      setBusy(0);
    }
  };
  const shut = async (port: number) => {
    setFailure("");
    try {
      await closePreview(machineId, port);
      reload();
    } catch (e) {
      setFailure(errMsg(e));
    }
  };
  const row = (l: Listener) => <PortRow key={l.port} listener={l} preview={openOn(open, l.port)} busy={busy === l.port} onOpen={() => void show(l.port)} onClose={() => void shut(l.port)} />;

  return (
    <Dialog title={workspaceName ? t("Previews of {workspace}", { workspace: workspaceName }) : t("Previews on {machine}", { machine: machineLabel })} icon="external-link" size="md" onClose={onClose}>
      <p className="modal-hint">{t("What the agents serve on {machine}, opened through this cockpit for this browser alone, in a tab of its own. A preview nobody uses for half an hour closes.", { machine: machineLabel })}</p>
      {(error || failure) && <Notice tone="error">{failure || error}</Notice>}
      {loading && !data && <Notice tone="hint">{t("Looking at the ports…")}</Notice>}
      {data && here.length + workspaces.length === 0 && <Notice tone="hint">{t("Nothing of yours listens on a port of {machine} now. Start the dev server in the workspace — npm run dev, python3 -m http.server — and it shows here.", { machine: machineLabel })}</Notice>}
      {listeners.length > 0 && (
        <ul className="pick-list preview-list">
          {here.length > 0 && (
            <li>
              <span className="pick-machine">{t("In this workspace")}</span>
              <ul>{here.map(row)}</ul>
            </li>
          )}
          {workspaces.length > 0 && (
            <li>
              <span className="pick-machine">{workspace ? t("In the other workspaces") : t("In the workspaces")}</span>
              <ul>{workspaces.map(row)}</ul>
            </li>
          )}
          {others.length > 0 && (
            <li>
              <button type="button" className="linkish preview-fold" aria-expanded={showOthers} onClick={() => setShowOthers((shown) => !shown)}>
                <Icon name={showOthers ? "chevron-down" : "chevron-right"} size={12} /> {t("{ports} outside the workspaces", { ports: plural(others.length, "other port") })}
              </button>
              {showOthers && <ul>{others.map(row)}</ul>}
            </li>
          )}
        </ul>
      )}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          {t("Close")}
        </button>
      </div>
    </Dialog>
  );
}

function PortRow({ listener, preview, busy, onOpen, onClose }: { listener: Listener; preview?: Preview; busy: boolean; onOpen: () => void; onClose: () => void }) {
  const where = whereWords(listener);
  return (
    <li className="pick-row preview-row" title={listener.dir}>
      <span className="pick-name">
        <span className="preview-port">{listenerWords(listener)}</span>
        {where && <span className="pick-hint"> · {where}</span>}
      </span>
      {preview && (
        <button type="button" className="linkish" onClick={onClose} title={t("Shut the preview at {address}", { address: hostOf(preview.url) })}>
          {t("Shut")}
        </button>
      )}
      <button type="button" className="btn sm" disabled={busy} onClick={onOpen} title={preview ? hostOf(preview.url) : undefined}>
        {busy ? t("Opening…") : t("Open")}
      </button>
    </li>
  );
}
