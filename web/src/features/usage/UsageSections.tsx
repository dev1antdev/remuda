// The Usage panel's Spend tab opens with these two sections: the plan limits of every agent's subscription, and the budgets.

import { actions } from "@/app/store";
import type { Machine } from "@/features/roster/api";
import { machineLabel } from "@/features/roster/options";
import { useNow } from "@/lib/clock";
import { t, tn } from "@/lib/i18n";
import { ago } from "@/lib/time";
import { Icon, Tip } from "@/ui";
import type { BudgetStatus } from "./api";
import { budgetTone, budgetWords, missingWords, ratio, scopeWords } from "./budgets";
import { accountWindows, agentName, claudeUnread, limitTone, resetTime, windowName } from "./limits";

// PlanLimits is the panel's first section: each agent's subscription windows, as full as their latest reading says, and
// where Claude Code runs with nobody reading its limits, the way to have them read (not for a viewer, who cannot)
export function PlanLimits({ machines, viewOnly }: { machines: Machine[]; viewOnly: boolean }) {
  const now = useNow(30_000);
  const windows = accountWindows(machines, now);
  const unread = viewOnly ? [] : machines.filter(claudeUnread);
  if (windows.length === 0 && unread.length === 0) return null;
  return (
    <section className="budgets plan-limits">
      <h3>{t("Plan limits")}</h3>
      {windows.length === 0 && <div className="empty">{t("No plan limits read yet: Codex reports its own after its next answer.")}</div>}
      {windows.map((w) => {
        const tone = limitTone(w.used);
        return (
          <div key={`${w.agent}|${w.name}`} className={`budget-row${tone ? ` is-${tone}` : ""}`} title={t("Read on {machine}, {ago}", { machine: w.machineLabel, ago: ago(w.at) })}>
            <span className="budget-scope">
              {agentName(w.agent)} · {windowName(w.name)}
              <span className="usage-row-sub"> · {t("resets {time}", { time: resetTime(w.resets_at, now) })}</span>
            </span>
            <span className="budget-bar" aria-hidden="true">
              <span style={{ width: `${Math.min(100, w.used)}%` }} />
            </span>
            <span className="budget-words">{Math.floor(w.used)} %</span>
          </div>
        );
      })}
      {unread.length > 0 && (
        <div className="empty">
          {t("Claude Code shows its plan limits to its status line alone, and remuda reads them there once you let it. Not set up yet on {machines}: the machine's ⓘ → Updates → “Show Claude Code's plan limits”.", { machines: unread.map((m) => m.label).join(", ") })}
        </div>
      )}
    </section>
  );
}

// Budgets is the panel's second section: every limit with a thin bar of its period's spend, and the way to add or edit one
// where the screen may change them (a viewer link's screen sees the limits only, and nothing when there are none)
export function Budgets({ budgets, machines, editable }: { budgets: BudgetStatus[]; machines: Machine[]; editable: boolean }) {
  const label = (id: string) => machineLabel(machines, id);
  if (!editable && budgets.length === 0) return null;
  return (
    <section className="budgets">
      <h3>
        {t("Budgets")}
        {editable && (
          <button type="button" className="btn sm" onClick={() => actions.openDialog({ kind: "budget", budget: null })}>
            <Icon name="plus" size={12} /> {t("Add a budget")}
          </button>
        )}
      </h3>
      {budgets.length === 0 && <div className="empty">{t("No budgets yet: a limit per day, week or month, on everything or one machine or folder. The usage pill turns amber at 80 %, a push comes at 80 % and at the limit.")}</div>}
      {budgets.map((b) => {
        const r = ratio(b);
        const tone = budgetTone(r);
        const missing = missingWords(b, label);
        return (
          <div key={b.id} className={`budget-row${tone ? ` is-${tone}` : ""}`} title={missing || undefined}>
            <span className="budget-scope">
              {scopeWords(b, label)}
              {b.note && <span className="usage-row-sub"> · {b.note}</span>}
              {missing && <span className="usage-row-sub"> · {tn(b.missing?.length ?? 0, "{n} machine not counted", "{n} machines not counted")}</span>}
            </span>
            <span className="budget-bar" aria-hidden="true">
              <span style={{ width: `${Math.min(100, 100 * r)}%` }} />
            </span>
            <span className="budget-words">{budgetWords(b)}</span>
            {editable && (
              <Tip text={t("Edit this budget")}>
                <button type="button" className="icon-btn sm" aria-label={t("Edit this budget")} onClick={() => actions.openDialog({ kind: "budget", budget: b })}>
                  <Icon name="edit" size={14} />
                </button>
              </Tip>
            )}
          </div>
        );
      })}
    </section>
  );
}
