// FormDialog is a dialog that is a form: fields, an error if the action failed, Cancel and the one filled button.

import type { ReactNode } from "react";
import { Dialog } from "./Dialog";
import { Notice } from "./Notice";
import type { IconName } from "@/ui/icons";

export type Submit = { err: string; busy: boolean; run: () => Promise<void> };

export function FormDialog({ title, icon, hint, children, onClose, submit, submitLabel, size }: {
  title: string;
  icon?: IconName;
  hint?: ReactNode;
  children: ReactNode;
  onClose: () => void;
  submit: Submit;
  submitLabel: string;
  size?: "sm" | "md" | "lg";
}) {
  return (
    <Dialog
      as="form"
      title={title}
      icon={icon}
      size={size}
      onClose={onClose}
      onSubmit={(e) => {
        e.preventDefault();
        void submit.run();
      }}
    >
      {hint && <p className="modal-hint">{hint}</p>}
      {children}
      {submit.err && <Notice tone="error">{submit.err}</Notice>}
      <div className="modal-actions">
        <button type="button" className="btn" onClick={onClose}>
          Cancel
        </button>
        <button type="submit" className="btn primary" disabled={submit.busy}>
          {submit.busy ? "…" : submitLabel}
        </button>
      </div>
    </Dialog>
  );
}
