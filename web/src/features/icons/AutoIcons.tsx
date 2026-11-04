// AutoIcons is the icon dialog's Automatic tab: the pictures the folder's project already keeps as its face and its
// owner's avatar, found on the folder's machine when the tab opens, each a tile to pick; nothing is set without a click.

import { useEffect, useState } from "react";
import { errMsg } from "@/lib/text";
import { Notice } from "@/ui";
import { t } from "@/lib/i18n";
import { findIconCandidates, type FoundIcons } from "./api";
import { fromWords, lookedWords, noteWords } from "./autoicon";

export function AutoIcons({ machineId, folder, busy, onUse }: { machineId: string; folder: string; busy: boolean; onUse: (image: string) => void }) {
  const [found, setFound] = useState<FoundIcons | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    let live = true;
    findIconCandidates(machineId, folder).then(
      (answer) => live && setFound(answer),
      (e) => live && setError(errMsg(e)),
    );
    return () => {
      live = false;
    };
  }, [machineId, folder]);
  if (error) return <Notice tone="error">{error}</Notice>;
  if (!found) return <p className="modal-hint">{t("Looking for the project's own icons…")}</p>;
  if (found.candidates.length === 0) {
    return (
      <div className="icon-source">
        <p className="modal-hint">{t("Nothing found in the project.")}</p>
        <p className="icon-looked">{lookedWords(found)}</p>
      </div>
    );
  }
  return (
    <div className="icon-source">
      <p className="modal-hint">{t("Pictures the project keeps, and its owner's avatar. Pick one to use it.")}</p>
      <div className="icon-candidates">
        {found.candidates.map((candidate) => (
          <button key={candidate.data} type="button" className="btn icon-candidate" disabled={busy} title={candidate.from} onClick={() => onUse(candidate.data)}>
            <img src={candidate.data} alt="" width={40} height={40} />
            <span>{fromWords(candidate)}</span>
          </button>
        ))}
      </div>
      {found.notes && found.notes.length > 0 && <p className="icon-looked">{noteWords(found.notes)}</p>}
    </div>
  );
}
