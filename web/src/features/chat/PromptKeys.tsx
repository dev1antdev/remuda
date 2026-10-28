// PromptKeys answer an agent's menu blind: up, down, Enter and Esc into the pane.

import { Icon } from "@/ui";
import { t } from "@/lib/i18n";

export function PromptKeys({ onKeys }: { onKeys: (keys: string[]) => void }) {
  return (
    <span className="prompt-card-keys">
      <button type="button" className="btn sm" title={t("Up")} aria-label={t("Up")} onClick={() => onKeys(["up"])}>
        <Icon name="chevron-up" size={14} />
      </button>
      <button type="button" className="btn sm" title={t("Down")} aria-label={t("Down")} onClick={() => onKeys(["down"])}>
        <Icon name="chevron-down" size={14} />
      </button>
      <button type="button" className="btn sm" onClick={() => onKeys(["enter"])}>
        {t("Enter")}
      </button>
      <button type="button" className="btn sm" onClick={() => onKeys(["esc"])}>
        {t("Esc")}
      </button>
    </span>
  );
}
