// The right end of a workspace card's second line, in Orca's order: the note, a plug while its checkout serves ports,
// and its pull request as one icon and number in the state's colour (cardPR.ts) — ⌘-click opens it on its site, a click
// opens its menu with a mouse and the card's details on a touch screen. Nothing here moves when a value changes: the ✓
// of a request ready to merge keeps its room.

import { useRef, type MouseEvent, type PointerEvent, type ReactNode } from "react";
import { actions } from "@/app/store";
import { openUrl } from "@/lib/host";
import { isMac } from "@/lib/prefs";
import { t } from "@/lib/i18n";
import { Icon, Menu, Tip, type MenuItem } from "@/ui";
import type { Machine, Session } from "./api";
import { noteDir, noteTip, portsWords, type Details } from "./cardFacts";
import { isLinkClick, moreTip, prLook, prNumber, prTip, type CardPR } from "./cardPR";
import { canRemoveWorktree } from "./prActions";
import { askRemoveWorktree, prMenuItems } from "./prMenu";

// tipLines draws a tooltip of several lines, a line each
export const tipLines = (text: string): ReactNode =>
  text.split("\n").map((line, i) => (
    <span key={i} className="tip-line">
      {line}
    </span>
  ));

export function CardBadges({ machine, session, name, details, editable, onDetails }: { machine: Machine; session: Session; name: string; details: Details; editable: boolean; onDetails: () => void }) {
  const { note, ports } = details;
  const [first, ...more] = details.prs;
  if (!note && !ports.length && !first) return null;
  const editNote = (e: MouseEvent) => {
    e.stopPropagation();
    actions.openDialog({ kind: "note", machineId: machine.id, dir: noteDir(session), name, text: note });
  };
  return (
    <span className="ws-tail">
      {note && (
        <Tip text={tipLines(noteTip(note))}>
          {editable ? (
            <button type="button" className="linkish ws-note" aria-label={t("Edit the note")} onClick={editNote}>
              <Icon name="note" size={12} />
            </button>
          ) : (
            <span className="ws-note" aria-label={t("Note")}>
              <Icon name="note" size={12} />
            </span>
          )}
        </Tip>
      )}
      {ports.length > 0 && (
        <Tip text={portsWords(ports)}>
          <span className="ws-ports" aria-label={portsWords(ports)}>
            <Icon name="plug" size={12} />
          </span>
        </Tip>
      )}
      {first && <PRBadge machine={machine} session={session} first={first} more={more} editable={editable} onDetails={onDetails} />}
    </span>
  );
}

// PRBadge is the card's first pull request and how many more its agents named
function PRBadge({ machine, session, first, more, editable, onDetails }: { machine: Machine; session: Session; first: CardPR; more: CardPR[]; editable: boolean; onDetails: () => void }) {
  const pressedBy = useRef("");
  const { pr, paneId } = first;
  const look = prLook(pr);
  const items = (): MenuItem[] => {
    const rows = prMenuItems(machine.id, paneId, pr);
    if (!editable) return rows.slice(0, 1); // a viewer opens the request and hands nothing to the agent
    if (canRemoveWorktree(session, pr)) rows.push({ sep: true }, { label: t("Remove the worktree and its branch…"), icon: "trash", danger: true, onSelect: () => askRemoveWorktree(machine, session) });
    return rows;
  };
  // a ⌘-click is the site's, a tap is the card's details: neither opens the menu, which Radix opens on the press
  const press = (e: PointerEvent<HTMLButtonElement>) => {
    pressedBy.current = e.pointerType;
    if (e.pointerType === "touch" || isLinkClick(e, isMac)) e.preventDefault();
  };
  const click = (e: MouseEvent<HTMLButtonElement>) => {
    e.stopPropagation();
    if (isLinkClick(e, isMac)) openUrl(pr.url);
    else if (pressedBy.current === "touch") onDetails();
  };
  return (
    <span className="ws-pr-group">
      <Menu
        label={t("{pr} actions", { pr: pr.label })}
        tip={tipLines(prTip(pr, isMac))}
        items={items}
        trigger={
          <button type="button" className={`linkish ws-pr tone-${look.tone}`} aria-label={`${pr.label} · ${look.label}`} onPointerDown={press} onClick={click}>
            <Icon name={look.icon} size={12} />
            <span className="ws-pr-num">{prNumber(pr)}</span>
            <Icon name="check" size={12} className={`ws-pr-ready${look.ready ? "" : " is-vacant"}`} />
          </button>
        }
      />
      {more.length > 0 && (
        <Tip text={tipLines(moreTip(more))}>
          <span className="ws-pr-more">+{more.length}</span>
        </Tip>
      )}
    </span>
  );
}
