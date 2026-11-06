// The listbox over the composer: the commands a slash offers or the files an @ offers, read from the pane's machine
// (kept a minute here as there) and ranked by what was typed. The keys are the composer's own; a row takes a click.

import { useEffect, useRef, useState } from "react";
import { errMsg } from "@/lib/text";
import { Icon } from "@/ui";
import { fetchCommands, fetchPaths } from "./api";
import { commandItems, pathItems, type Command, type Item, type Token } from "./complete";
import { t } from "@/lib/i18n";

const KEEP = 60_000;
type Loaded = { commands?: Command[]; paths?: string[]; cut?: boolean };
const loads = new Map<string, { at: number; promise: Promise<Loaded> }>();

// load reads one list for a folder, once a minute at most per page
function load(kind: Token["kind"], machineId: string, dir: string, agent: string): Promise<Loaded> {
  const key = `${kind}|${machineId}|${dir}|${kind === "command" ? agent : ""}`;
  const had = loads.get(key);
  if (had && Date.now() - had.at < KEEP) return had.promise;
  const promise = kind === "command" ? fetchCommands(machineId, dir, agent).then((commands) => ({ commands })) : fetchPaths(machineId, dir).then((r) => ({ paths: r.paths, cut: r.cut }));
  promise.catch(() => loads.delete(key));
  loads.set(key, { at: Date.now(), promise });
  return promise;
}

export type Completion = { items: Item[]; cut: boolean; error: string | null };

// useCompletion is the list for the token under the caret, empty while nothing is typed there
export function useCompletion(machineId: string, dir: string, agent: string, token: Token | null): Completion {
  const [loaded, setLoaded] = useState<{ kind: Token["kind"]; dir: string; data: Loaded } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const kind = token?.kind ?? null;
  useEffect(() => {
    if (!kind || !dir) return;
    let gone = false;
    setError(null);
    load(kind, machineId, dir, agent).then(
      (data) => !gone && setLoaded({ kind, dir, data }),
      (e) => !gone && setError(errMsg(e)),
    );
    return () => {
      gone = true;
    };
  }, [kind, machineId, dir, agent]);
  if (!token || !loaded || loaded.kind !== token.kind || loaded.dir !== dir) return { items: [], cut: false, error: token ? error : null };
  const items = token.kind === "command" ? commandItems(loaded.data.commands ?? [], token.query) : pathItems(loaded.data.paths ?? [], token.query);
  return { items, cut: !!loaded.data.cut, error };
}

// CompletionList draws the rows; the active one follows the keys and the mouse, a mousedown picks so the field keeps its focus
export function CompletionList({ kind, items, active, cut, error, onPick, onHover }: {
  kind: Token["kind"];
  items: Item[];
  active: number;
  cut: boolean;
  error: string | null;
  onPick: (item: Item) => void;
  onHover: (index: number) => void;
}) {
  const box = useRef<HTMLDivElement>(null);
  useEffect(() => {
    box.current?.querySelector<HTMLElement>(".complete-item.is-active")?.scrollIntoView({ block: "nearest" });
  }, [active, items]);
  if (items.length === 0 && !error) return null;
  return (
    <div ref={box} className="complete" role="listbox" aria-label={kind === "command" ? t("Commands") : t("Files")}>
      {items.map((item, i) => (
        <div
          key={item.id}
          role="option"
          aria-selected={i === active}
          className={`complete-item${i === active ? " is-active" : ""}`}
          onMouseDown={(e) => {
            e.preventDefault();
            onPick(item);
          }}
          onMouseEnter={() => onHover(i)}
        >
          <span className="complete-icon">
            <Icon name={kind === "command" ? "terminal" : "file"} size={14} />
          </span>
          <span className="complete-text">
            <span className="complete-title">{item.title}</span>
            {item.hint && <small>{item.hint}</small>}
          </span>
          {item.tag && <span className="complete-tag">{item.tag}</span>}
        </div>
      ))}
      {error ? <div className="complete-note is-error">{error}</div> : cut && <div className="complete-note">{t("Only the folder's first files are listed; type more of the name")}</div>}
      <div className="complete-keys">
        <kbd>↑</kbd> <kbd>↓</kbd> {t("walk")} · <kbd>↵</kbd> <kbd>Tab</kbd> {t("pick")} · <kbd>Esc</kbd> {t("close")}
      </div>
    </div>
  );
}
