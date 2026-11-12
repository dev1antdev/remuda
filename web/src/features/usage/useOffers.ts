// The open offers of the plan limits, read whenever the roster's list of their ids moves — never on a viewer's page,
// which the server would refuse — and the two answers the person gives one: run it on the other vendor, or let it be.

import { act } from "@/app/act";
import { actions } from "@/app/store";
import { useRoster } from "@/features/roster/useRoster";
import { t } from "@/lib/i18n";
import { useQuery } from "@/lib/query";
import { errMsg } from "@/lib/text";
import { declineOffer, fetchOffers, runOffer } from "./api";
import { agentLabel, whatWaits, type PlanOffer } from "./offers";

export function useOffers(): PlanOffer[] {
  const { offers: ids, viewOnly } = useRoster();
  const key = viewOnly ? "" : (ids ?? []).join(",");
  const { data } = useQuery<PlanOffer[]>(async () => (key ? (await fetchOffers()).offers : []), [key], { enabled: !viewOnly });
  return key ? (data ?? []) : [];
}

// takeOffer runs the held work on the other vendor's agent now, and says how it went
export async function takeOffer(o: PlanOffer): Promise<void> {
  try {
    await runOffer(o.id);
    actions.notify(t("{what} started on {agent}.", { what: whatWaits(o), agent: agentLabel(o.to) }));
  } catch (e) {
    actions.notify(t("{what} did not start on {agent}: {why}", { what: whatWaits(o), agent: agentLabel(o.to), why: errMsg(e) }), { error: true });
  }
  await act(Promise.resolve());
}

// letOfferBe declines the offer for every device until its window resets
export const letOfferBe = (o: PlanOffer): Promise<void> => act(declineOffer(o.id));
