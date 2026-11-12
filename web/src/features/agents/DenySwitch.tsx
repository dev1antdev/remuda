// DenySwitch is the checkbox "Don't wait for permissions: deny and carry on" with what it means under it, or, disabled,
// why the agents it covers cannot (deny.ts).

import { t } from "@/lib/i18n";
import { denyHint } from "./deny";

export function DenySwitch({ checked, onChange, why, hint, className = "check-row", hintClass = "modal-hint" }: {
  checked: boolean;
  onChange: (on: boolean) => void;
  why: string; // whyNoDeny's reason, "" when the switch applies
  hint?: string; // what it means here, when the usual words (denyHint) do not fit
  className?: string; // the form's own checkbox row
  hintClass?: string; // the form's own note under a field
}) {
  return (
    <div className="deny-switch">
      <label className={className}>
        <input type="checkbox" checked={checked && !why} disabled={!!why} onChange={(e) => onChange(e.target.checked)} />
        {t("Don't wait for permissions: deny and carry on")}
      </label>
      <p className={hintClass}>{why || hint || denyHint()}</p>
    </div>
  );
}
