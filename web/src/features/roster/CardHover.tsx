// A workspace card's hover card, after Orca's: resting on the card opens its details beside it (ui/HoverCard.tsx), and
// opening them asks gh on the machine for its pull requests again, so the open card shows the checks as they stand. A
// touch screen has no hover: the card's ⋯ menu and a tap on its pull request open the same details as a popover.

import { useEffect, useMemo, type ReactNode } from "react";
import { isTouch } from "@/lib/prefs";
import HoverCard, { useHoverCard } from "@/ui/HoverCard";
import type { Machine, Session } from "./api";
import { refreshPRs } from "./cardApi";
import { dropDetails, askDetails, useDetailsAsked } from "./cardAsk";
import { CardDetails } from "./CardDetails";
import { detailsOf, type Details } from "./cardFacts";

export type CardHoverOf = {
  details: Details;
  hoverable: boolean; // the card opens on a rest: the row then wears no title of its own
  trigger: ReturnType<typeof useHoverCard>["trigger"];
  node: ReactNode; // the card, placed inside the row
  ask: () => void; // the details opened by hand, on a touch screen
};

// useCardHover is a workspace card's details and the hover card that shows them; tip is the row's own tooltip, whose
// lines the card shows above its sections, less the request a section of its own names
export function useCardHover({ machine, session, leadPaneId, hidden, editable, name, tip }: { machine: Machine; session: Session; leadPaneId?: string; hidden: boolean; editable: boolean; name: string; tip: string }): CardHoverOf {
  const details = useMemo(() => detailsOf(session, leadPaneId, hidden), [session, leadPaneId, hidden]);
  const asked = useDetailsAsked(machine.id, session.id);
  const card = useHoverCard(details.any && !isTouch);
  const open = details.any && (card.open || asked);
  const panes = details.prs.map((p) => p.paneId).join(" ");
  useEffect(() => {
    if (open && editable && panes) refreshPRs(machine.id, panes.split(" ")).catch(() => {}); // a card that could not ask shows the state it has, and how old it is
  }, [open, editable, machine.id, panes]);
  const about = useMemo(() => tip.split("\n").filter((line) => line && !details.prs.some(({ pr }) => line.includes(pr.url))), [tip, details.prs]);
  const close = () => {
    card.pin(false);
    dropDetails();
  };
  const node = details.any ? (
    <HoverCard open={open} onClose={close} side={isTouch ? "bottom" : "right"}>
      <CardDetails machine={machine} session={session} name={name} details={details} about={about} editable={editable} />
    </HoverCard>
  ) : null;
  return { details, hoverable: details.any && !isTouch, trigger: card.trigger, node, ask: () => askDetails(machine.id, session.id) };
}
