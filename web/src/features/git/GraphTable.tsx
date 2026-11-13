// The graph's table: the columns' heads, a row per commit (the uncommitted changes first when the tree is dirty), and
// "Load 300 more" at the end. One set of handlers for every row: a click picks a commit, a double-click on a branch's
// label checks it out, a right-click (or a ⋯, or a label tapped on a touch screen) opens the menu for what is under it.

import type { CSSProperties, MouseEvent } from "react";
import { isTouch } from "@/lib/prefs";
import type { Ref } from "./api";
import { GraphRow, lanesWidth } from "./GraphRow";
import { PAGE, rowCommit, type History } from "./history";
import { t, tc } from "@/lib/i18n";

// Pressed is what a press landed on: a row by its index, and a label's part on it when one was under the pointer
export type Pressed = { index: number; ref: { kind: Ref["kind"]; name: string } | null };

export type MenuPoint = { x: number; y: number };

function pressedOf(e: MouseEvent): Pressed | null {
  const target = e.target as HTMLElement;
  const row = target.closest<HTMLElement>("[data-index]");
  if (!row) return null;
  const part = target.closest<HTMLElement>("[data-ref-name]");
  const ref = part ? { kind: part.dataset.refKind as Ref["kind"], name: part.dataset.refName! } : null;
  return { index: Number(row.dataset.index), ref };
}

// under is the point below an element, for a menu opened from it
function under(el: Element): MenuPoint {
  const r = el.getBoundingClientRect();
  return { x: r.left, y: r.bottom };
}

export function GraphTable({ history, colors, remotes, selected, now, dirty, menus, more, onPick, onMenu, onCheckout, onMore }: {
  history: History;
  colors: string[];
  remotes: boolean;
  selected: string | null;
  now: number;
  dirty: number;
  menus: boolean; // the person's page: menus and checkouts; a viewer's reads only
  more: { busy: boolean; error: string };
  onPick: (index: number) => void;
  onMenu: (pressed: Pressed, at: MenuPoint) => void;
  onCheckout: (ref: { kind: Ref["kind"]; name: string }) => void;
  onMore: () => void;
}) {
  const width = lanesWidth(history.lanes.width);
  const style = { "--git-lanes-w": `${width}px` } as CSSProperties;
  const click = (e: MouseEvent) => {
    const pressed = pressedOf(e);
    if (!pressed) return;
    const button = (e.target as HTMLElement).closest("[data-more]");
    if (menus && button) return onMenu({ index: pressed.index, ref: null }, under(button));
    const part = (e.target as HTMLElement).closest("[data-ref-name]");
    if (menus && isTouch && part && pressed.ref && pressed.ref.kind !== "head") return onMenu(pressed, under(part));
    onPick(pressed.index);
  };
  const doubleClick = (e: MouseEvent) => {
    const pressed = pressedOf(e);
    if (menus && pressed?.ref && (pressed.ref.kind === "branch" || pressed.ref.kind === "remote")) onCheckout(pressed.ref);
  };
  const contextMenu = (e: MouseEvent) => {
    const pressed = pressedOf(e);
    if (!menus || !pressed) return;
    e.preventDefault();
    onMenu(pressed.ref?.kind === "head" ? { ...pressed, ref: null } : pressed, { x: e.clientX, y: e.clientY });
  };
  return (
    <div className="git-table" role="table" aria-label={t("Commits")} style={style}>
      <div className="git-row git-cols" role="row">
        <span className="git-graph" role="columnheader">
          {t("Graph")}
        </span>
        <span className="git-desc" role="columnheader">
          {t("Description")}
        </span>
        <span className="git-date" role="columnheader">
          {t("Date")}
        </span>
        <span className="git-author" role="columnheader">
          {t("Author")}
        </span>
        <span className="git-hash" role="columnheader">
          {tc("git", "Commit")}
        </span>
        {menus && isTouch && <span className="git-row-more" />}
      </div>
      <div className="git-rows" role="rowgroup" onClick={click} onDoubleClick={doubleClick} onContextMenu={contextMenu}>
        {history.rows.map((row, index) => {
          const commit = rowCommit(history, index);
          return <GraphRow key={commit?.hash ?? "*"} index={index} commit={commit} row={row} width={width} colors={colors} remotes={remotes} selected={!!commit && commit.hash === selected} now={now} menu={menus && isTouch} dirty={dirty} />;
        })}
        {history.log.more && (
          <div className="git-more">
            <button type="button" className="linkish" disabled={more.busy} onClick={onMore}>
              {more.busy ? t("Loading more commits…") : t("Load {n} more", { n: PAGE })}
            </button>
            {more.error && <span className="git-error-note">{more.error}</span>}
          </div>
        )}
      </div>
    </div>
  );
}
