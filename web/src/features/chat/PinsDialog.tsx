// The pins of a pane's conversations: every turn bookmarked in this folder, newest first, each a jump back to it — into
// the session shown, or an older session of the folder through the lens's own picker.

import { actions } from "@/app/store";
import { usePref } from "@/lib/prefs";
import { Dialog, Icon, Notice, Tip } from "@/ui";
import { pinKey, pinWords, type Pin } from "./pins";
import { t } from "@/lib/i18n";

export default function PinsDialog({ machineId, paneId, dir, shown, title, onClose }: {
  machineId: string;
  paneId: string;
  dir: string;
  shown: string | null; // the session on screen, whose pins need no session named
  title: string;
  onClose: () => void;
}) {
  const [pins, setPins] = usePref<Pin[]>(pinKey(machineId, dir), []);
  const go = (pin: Pin) => {
    onClose();
    actions.jumpTo(machineId, paneId, pin.session, pin.turn);
  };
  const unpin = (pin: Pin) => setPins((prev) => prev.filter((p) => p.turn !== pin.turn));
  const clear = () => actions.ask(`Remove all ${pins.length} pins of ${title}? The turns themselves stay.`, t("Remove all"), () => setPins([]));
  return (
    <Dialog title={t("Pins")} icon="pin" size="md" onClose={onClose}>
      <h3 className="standup-title">{title}</h3>
      {pins.length === 0 ? (
        <Notice tone="hint">{t("Nothing pinned here yet. The pin beside a turn's link keeps that turn in this list.")}</Notice>
      ) : (
        <ul className="pins-list" aria-label={t("Pinned turns")}>
          {pins.map((pin) => (
            <li key={pin.turn} className="pins-item">
              <button type="button" className="pins-row" onClick={() => go(pin)} title={t("Open the conversation at this turn")}>
                <Icon name={pin.kind === "prompt" ? "user" : "sparkles"} size={12} className="pins-kind" />
                <span className="pins-line">{pin.line}</span>
                <span className="pins-when">{pinWords(pin, shown)}</span>
              </button>
              <Tip text="Unpin">
                <button type="button" className="icon-btn sm" aria-label={t("Unpin this turn")} onClick={() => unpin(pin)}>
                  <Icon name="x" size={12} />
                </button>
              </Tip>
            </li>
          ))}
        </ul>
      )}
      {pins.length > 0 && (
        <div className="modal-actions">
          <button type="button" className="btn" onClick={clear}>
            <Icon name="trash" size={14} /> Remove all
          </button>
        </div>
      )}
    </Dialog>
  );
}
