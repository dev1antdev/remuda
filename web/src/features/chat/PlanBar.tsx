// PlanBar is the agent's plan over the chat: one quiet line with how far it is and what it is on, unfolding to the list
// as the agent keeps it. Nothing when the agent keeps none.

import { useState } from "react";
import { Icon } from "@/ui";
import type { IconName } from "@/ui/icons";
import type { PlanItem } from "./api";
import { counts, current, currentWords, progressWords } from "./plan";
import { t } from "@/lib/i18n";

export const PLAN_MARK: Record<PlanItem["status"], IconName> = { pending: "square", in_progress: "play", completed: "check" };

export function PlanBar({ plan }: { plan?: PlanItem[] }) {
  const [open, setOpen] = useState(false);
  if (!plan?.length) return null;
  const now = current(plan);
  const left = counts(plan).open;
  return (
    <details className="plan-bar" open={open} onToggle={(e) => setOpen((e.target as HTMLDetailsElement).open)}>
      <summary title={t("The agent's own task list, as it keeps it")}>
        <Icon name="list" size={12} className="plan-icon" /> {t("Plan")} <span className="plan-count">{progressWords(plan)}</span>
        {now ? <span className="plan-now">· {currentWords(now)}</span> : left === 0 ? <span className="plan-now">{t("· done")}</span> : null}
      </summary>
      {open && (
        <ul className="plan-list">
          {plan.map((item) => (
            <li key={item.id} className={`plan-item is-${item.status}`} title={item.status === "in_progress" && item.active ? item.active : undefined}>
              <Icon name={PLAN_MARK[item.status]} size={12} className="plan-mark" />
              <span className="plan-text">{item.text}</span>
            </li>
          ))}
        </ul>
      )}
    </details>
  );
}
