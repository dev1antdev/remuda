// FindTool is the find bar of the chat lens, in the tab strip's tools slot: its toggle keeps its place and the
// bar — a field, "3 of 12", the two steps and a close — opens to the toggle's left, so nothing else in the strip
// moves. The chat does the matching (useFind); the store carries the query and the count between the two.

import { useEffect, useRef, type KeyboardEvent } from "react";
import { actions, app } from "@/app/store";
import { isMac } from "@/lib/prefs";
import { Icon, Tip } from "@/ui";
import { countWords } from "./find";

const SHORTCUT = isMac ? "⌘F" : "Ctrl+F";

export function FindTool() {
  const find = app.use((s) => s.find);
  const field = useRef<HTMLInputElement>(null);
  // every ⌘F puts the focus back in the field, with the query selected for a new one
  useEffect(() => {
    if (!find) return;
    field.current?.focus();
    field.current?.select();
  }, [find?.opened]); // eslint-disable-line react-hooks/exhaustive-deps
  const onKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "Enter") {
      e.preventDefault();
      actions.stepFind(e.shiftKey ? -1 : 1);
    } else if (e.key === "Escape") {
      e.preventDefault();
      actions.closeFind();
    } else if (e.code === "KeyF" && (isMac ? e.metaKey : e.ctrlKey)) {
      e.preventDefault();
      e.currentTarget.select();
    }
  };
  return (
    <>
      {find && (
        <span className="find-bar" role="search">
          <input
            ref={field}
            className="sm"
            type="text"
            value={find.query}
            placeholder="Find"
            aria-label="Find in this conversation"
            autoComplete="off"
            spellCheck={false}
            onChange={(e) => actions.setFindQuery(e.target.value)}
            onKeyDown={onKey}
          />
          <span className="find-count" aria-live="polite">
            {find.query ? countWords(find.at, find.total) : ""}
          </span>
          <button type="button" className="icon-btn sm" title="Previous match (Shift+Enter)" aria-label="Previous match" disabled={find.total === 0} onClick={() => actions.stepFind(-1)}>
            <Icon name="chevron-up" size={14} />
          </button>
          <button type="button" className="icon-btn sm" title="Next match (Enter)" aria-label="Next match" disabled={find.total === 0} onClick={() => actions.stepFind(1)}>
            <Icon name="chevron-down" size={14} />
          </button>
          <button type="button" className="icon-btn sm" title="Close (Esc)" aria-label="Close the find bar" onClick={actions.closeFind}>
            <Icon name="x" size={14} />
          </button>
        </span>
      )}
      <Tip text={`Find in this conversation (${SHORTCUT})`}>
        <button type="button" className={`icon-btn sm find-toggle${find ? " on" : ""}`} aria-label="Find in this conversation" aria-pressed={!!find} onClick={() => (find ? actions.closeFind() : actions.openFind())}>
          <Icon name="search" size={14} />
        </button>
      </Tip>
    </>
  );
}
