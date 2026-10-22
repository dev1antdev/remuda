// The command palette on ⌘K: a line to type in, every agent as a row to jump to, every command the page
// offers, ranked as you type; ↑ ↓ move, ↵ runs, Esc closes. What it lists comes from app/commands.

import { useEffect, useMemo, useState, type KeyboardEvent } from "react";
import { Dialog, Icon, Logo } from "@/ui";
import { brandColor } from "@/ui/brand";
import { rank, type Entry } from "./rank";

export default function Palette({ entries, onClose }: { entries: (query: string) => Entry[]; onClose: () => void }) {
  const [query, setQuery] = useState("");
  const [index, setIndex] = useState(0);
  const list = useMemo(() => {
    const all = entries(query);
    const typed = query.trim();
    const of = (kind: Entry["kind"]) => rank(all.filter((e) => e.kind === kind && !e.always), typed);
    return [...of("pane"), ...of("command"), ...all.filter((e) => e.always)];
  }, [entries, query]);
  const at = Math.min(index, Math.max(0, list.length - 1));
  useEffect(() => {
    document.getElementById(`palette-${at}`)?.scrollIntoView({ block: "nearest" });
  }, [at]);

  const pick = (entry: Entry) => {
    onClose();
    entry.run();
  };
  const onKey = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === "ArrowDown" || e.key === "ArrowUp") {
      e.preventDefault();
      if (list.length) setIndex((at + (e.key === "ArrowDown" ? 1 : list.length - 1)) % list.length);
    } else if (e.key === "Enter") {
      e.preventDefault();
      const entry = list[at];
      if (entry) pick(entry);
    }
  };

  let lastKind: Entry["kind"] | "" = "";
  return (
    <Dialog onClose={onClose} className="palette" labelledBy="palette-label">
      <div className="palette-head">
        <Icon name="search" size={16} className="palette-head-icon" />
        <input
          id="palette-label"
          autoFocus
          role="combobox"
          aria-expanded="true"
          aria-controls="palette-list"
          aria-activedescendant={list.length ? `palette-${at}` : undefined}
          aria-label="Jump to an agent or run a command"
          placeholder="Jump to an agent or run a command…"
          spellCheck={false}
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setIndex(0);
          }}
          onKeyDown={onKey}
        />
      </div>
      <div className="palette-list" id="palette-list" role="listbox">
        {list.map((entry, i) => {
          const head = entry.kind !== lastKind && !entry.always ? <div className="palette-group">{entry.kind === "pane" ? "Agents" : "Commands"}</div> : null;
          lastKind = entry.always ? "" : entry.kind;
          return (
            <Row key={entry.id} entry={entry} id={`palette-${i}`} selected={i === at} head={head} onHover={() => setIndex(i)} onPick={() => pick(entry)} />
          );
        })}
      </div>
      <div className="palette-foot">
        <span>
          <kbd>↑</kbd>
          <kbd>↓</kbd> move
        </span>
        <span>
          <kbd>↵</kbd> run
        </span>
        <span>
          <kbd>esc</kbd> close
        </span>
      </div>
    </Dialog>
  );
}

function Row({ entry, id, selected, head, onHover, onPick }: { entry: Entry; id: string; selected: boolean; head: React.ReactNode; onHover: () => void; onPick: () => void }) {
  const pane = entry.pane;
  const markStyle = pane && pane.display === "working" && !pane.alert && pane.agent ? { color: brandColor(pane.agent) } : undefined;
  return (
    <>
      {head}
      <div id={id} role="option" aria-selected={selected} className={`palette-row${entry.danger ? " danger" : ""}`} onMouseMove={onHover} onClick={onPick}>
        {pane ? (
          <>
            <span className={`radar-mark ${pane.alert ? "alert" : pane.display}`} style={markStyle}>
              {pane.mark}
            </span>
            {pane.agent ? (
              <Logo agent={pane.agent} />
            ) : (
              <span className="palette-icon">
                <Icon name="terminal" size={14} />
              </span>
            )}
          </>
        ) : (
          <span className="palette-icon">
            <Icon name={entry.icon ?? "zap"} size={14} />
          </span>
        )}
        <span className="palette-title">{entry.title}</span>
        {entry.hint && <span className="palette-hint">{entry.hint}</span>}
        {pane?.current ? <span className="palette-detail">here now</span> : entry.detail ? <span className="palette-detail">{entry.detail}</span> : null}
      </div>
    </>
  );
}
