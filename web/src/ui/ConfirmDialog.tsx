// A yes/no before something destructive; focus starts on Cancel.

import { Dialog } from "./Dialog";
import { t } from "@/lib/i18n";

export function ConfirmDialog({ text, label, onClose, onConfirm }: { text: string; label: string; onClose: () => void; onConfirm: () => void }) {
  return (
    <Dialog onClose={onClose}>
      <p className="confirm-text">{text}</p>
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          {t("Cancel")}
        </button>
        <button type="button" className="btn danger" onClick={onConfirm}>
          {label}
        </button>
      </div>
    </Dialog>
  );
}
