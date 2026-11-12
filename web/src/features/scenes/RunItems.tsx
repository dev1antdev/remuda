// A run's items as they stand: each step or action with its mark and its outcome, and Open for a step's new pane.

import type { Recipe } from "@/features/recipes/api";
import type { AgentKind, Machine } from "@/features/roster/api";
import { actions } from "@/app/store";
import { useSharing } from "@/features/roster/useSharing";
import { Icon } from "@/ui";
import type { Run } from "./api";
import { actionWords, itemTone, itemWords, stepWhere, stepWords } from "./scenes";
import { t } from "@/lib/i18n";

const MARKS = { ok: "circle-check", bad: "alert-triangle", busy: "refresh-cw", quiet: "clock" } as const;

export default function RunItems({ run, machines, agents, recipes }: { run: Run; machines: Machine[]; agents: AgentKind[]; recipes: Recipe[] }) {
  const now = new Date(run.at).getTime();
  const share = useSharing(); // under the screen-sharing mode the hidden machines and folders by their neutral names
  return (
    <ul className="pick-list scene-outcomes">
      {run.items.map((item) => {
        const tone = itemTone(item);
        const machine = item.step?.machine ?? item.action?.machine ?? "";
        const what = item.step ? `${stepWhere(item.step, machines)} · ${stepWords(item.step, agents, recipes)}` : item.action?.kind ? actionWords(item.action, agents, now) : (item.action?.id ?? "");
        return (
          <li key={`${item.index}|${item.action?.id ?? ""}`} className={`pick-row scene-outcome is-${tone}`}>
            <Icon name={MARKS[tone]} size={14} />
            <span className="pick-name">
              {share.redact(what)}
              <small>{share.redact(itemWords(item))}</small>
            </span>
            {item.pane && (
              <button type="button" className="btn sm" onClick={() => actions.openPane(machine, item.pane ?? "")}>
                {t("Open")}
              </button>
            )}
          </li>
        );
      })}
    </ul>
  );
}
