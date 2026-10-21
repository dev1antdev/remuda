// PromptKeys answer an agent's menu blind: up, down, Enter and Esc into the pane.

import { Icon } from "@/ui";

export function PromptKeys({ onKeys }: { onKeys: (keys: string[]) => void }) {
  return (
    <span className="prompt-card-keys">
      <button type="button" className="btn sm" title="Up" aria-label="Up" onClick={() => onKeys(["up"])}>
        <Icon name="chevron-up" size={14} />
      </button>
      <button type="button" className="btn sm" title="Down" aria-label="Down" onClick={() => onKeys(["down"])}>
        <Icon name="chevron-down" size={14} />
      </button>
      <button type="button" className="btn sm" onClick={() => onKeys(["enter"])}>
        Enter
      </button>
      <button type="button" className="btn sm" onClick={() => onKeys(["esc"])}>
        Esc
      </button>
    </span>
  );
}
