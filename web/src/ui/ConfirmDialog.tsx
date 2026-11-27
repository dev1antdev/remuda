// A yes/no before something destructive; focus starts on Cancel.

import { useId } from "react";
import { Dialog } from "./Dialog";
import { t } from "@/lib/i18n";

export function ConfirmDialog({ text, label, onClose, onConfirm }: { text: string; label: string; onClose: () => void; onConfirm: () => void }) {
  const id = useId();
  return (
    <Dialog onClose={onClose} labelledBy={id}>
      <p className="confirm-text" id={id}>
        {text}
      </p>
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
