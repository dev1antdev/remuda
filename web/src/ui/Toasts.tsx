// Transient status lives in a corner, never in a bar: a pill in the header would move everything beside it.

import type { ReactNode } from "react";
import { createPortal } from "react-dom";
import Icon from "@/ui/icons";
import type { Tone } from "./Notice";
import { t as tr } from "@/lib/i18n";

export type ToastItem = { id: string; tone: Tone; text: ReactNode; action?: ReactNode; onDismiss?: () => void; busy?: boolean };

export function Toasts({ items }: { items: ToastItem[] }) {
  if (items.length === 0) return null;
  return createPortal(
    <div className="toasts" role="status" aria-live="polite">
      {items.map((t) => (
        <div key={t.id} className={`toast ${t.tone}`}>
          {t.busy && <span className="spinner" aria-hidden="true" />}
          <span className="toast-text">{t.text}</span>
          {t.action}
          {t.onDismiss && (
            <button type="button" className="icon-btn sm" onClick={t.onDismiss} title={tr("Dismiss")} aria-label={tr("Dismiss")}>
              <Icon name="x" size={12} />
            </button>
          )}
        </div>
      ))}
    </div>,
    document.body,
  );
}
