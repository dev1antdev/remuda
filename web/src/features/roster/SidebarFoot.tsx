// The sidebar's foot, after Orca's: "3 agents · 2 need you" across the connected machines, the second part a link to
// the agent that has waited longest. Absent while no agent runs anywhere.

import { actions } from "@/app/store";
import { Tip } from "@/ui";
import { agentsWords, needWords, type Tally, type Waiting } from "./sidebarAgents";
import { t } from "@/lib/i18n";

export function SidebarFoot({ tally, oldest }: { tally: Tally; oldest: Waiting | undefined }) {
  if (tally.agents === 0) return null;
  return (
    <div className="sidebar-foot">
      <span>{agentsWords(tally.agents)}</span>
      {tally.needs > 0 && oldest && (
        <>
          <span aria-hidden="true">·</span>
          <Tip text={t("Go to the one that has waited longest")} side="top">
            <button type="button" className="linkish sidebar-foot-needs" onClick={() => actions.openPane(oldest.machine.id, oldest.pane.id)}>
              {needWords(tally.needs)}
            </button>
          </Tip>
        </>
      )}
    </div>
  );
}
