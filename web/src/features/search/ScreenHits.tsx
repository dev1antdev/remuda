// The search panel's Screens part: where the text stood on an agent's screen in the recorded turns of every machine —
// each recording's first appearance with the rows around it, how long it stayed, how often it came back — a click
// opening the player at that frame. Under the screen-sharing mode a hidden folder's are left out, as the player's list
// leaves them; a recording of a machine the fleet no longer has is not listed, since nothing can play it.

import { useState } from "react";
import type { Machine } from "@/features/roster/api";
import { machineLabel } from "@/features/roster/options";
import { useSharing } from "@/features/roster/useSharing";
import type { ScreenHit } from "@/features/terminal/timelapse/api";
import { excerptRows, momentOf, stayWords, unreadWords } from "@/features/terminal/timelapse/found";
import { useScreenSearch } from "@/features/terminal/timelapse/useScreenSearch";
import { shownRecordings } from "@/features/terminal/timelapse/words";
import { t, tn } from "@/lib/i18n";
import { basename } from "@/lib/text";
import { whenShort } from "@/lib/time";
import { Icon, Notice } from "@/ui";
import { highlight } from "./highlight";

// FOLDED is how many hits the part shows before "Show N more"
const FOLDED = 3;

type Found = { query: string; hits: ScreenHit[]; note: string; error: string };

// useScreenHits is the screens' answer to the panel's query and filters, less what the page cannot or must not show
export function useScreenHits(query: string, machines: Machine[], scope: { machine: string; dir: string; days: number }): Found {
  const share = useSharing();
  const found = useScreenSearch(query, scope);
  const known = (found.data?.hits ?? []).filter((hit) => machines.some((m) => m.id === hit.machine));
  const hits = found.query ? shownRecordings(known, { machine: share.machineHidden, folder: share.folderHidden }) : [];
  return { query: found.query, hits, note: found.query && found.data ? unreadWords(found.data) : "", error: found.query ? found.error : "" };
}

export function ScreenHits({ found, machines, onOpen }: { found: Found; machines: Machine[]; onOpen: (hit: ScreenHit, query: string) => void }) {
  const [open, setOpen] = useState(false);
  const { hits, note } = found;
  if (!hits.length && !note && !found.error) return null;
  const shown = open ? hits : hits.slice(0, FOLDED);
  return (
    <section className="search-screens" aria-label={t("Screens")}>
      <div className="search-part-head">
        <Icon name="film" size={14} />
        <span>{t("Screens")}</span>
        {hits.length > 0 && <span className="badge quiet">{hits.length}</span>}
        <span className="search-part-hint">{t("what stood on the agents' screens in their recorded turns")}</span>
      </div>
      {found.error && <Notice tone="error">{found.error}</Notice>}
      {shown.map((hit) => (
        <button key={hit.recording} type="button" className="search-hit search-screen-hit" title={hit.dir} onClick={() => onOpen(hit, found.query)}>
          <span className="search-hit-who">
            <Icon name="film" size={14} />
          </span>
          <span className="search-hit-body">
            <span className="search-hit-meta">
              <b>{hit.workspace || basename(hit.dir)}</b>
              {machines.length > 1 && <span> · {machineLabel(machines, hit.machine)}</span>}
              <span className="search-hit-when"> · {whenShort(momentOf(hit))}</span>
              <span className="search-hit-when"> · {stayWords(hit)}</span>
            </span>
            <span className="search-screen">
              {excerptRows(hit).map((row, i) => (
                <span key={i} className={row.match ? "search-screen-row is-match" : "search-screen-row"}>
                  {row.match ? highlight(row.text, found.query) : row.text || " "}
                </span>
              ))}
            </span>
          </span>
        </button>
      ))}
      {hits.length > FOLDED && (
        <button type="button" className="linkish search-part-more" onClick={() => setOpen(!open)}>
          {open ? t("Show less") : tn(hits.length - FOLDED, "Show {n} more", "Show {n} more")}
        </button>
      )}
      {note && <Notice tone="hint">{note}</Notice>}
    </section>
  );
}
