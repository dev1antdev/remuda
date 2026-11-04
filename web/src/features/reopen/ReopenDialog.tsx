// The agents whose panes went away, newest first: a click — or ↑ ↓ and Enter — starts one again on its own session,
// in a tab of the workspace on its folder, or a new workspace when that is gone.

import { useRef, useState, type KeyboardEvent } from "react";
import { machineLabel } from "@/features/roster/options";
import { useRoster } from "@/features/roster/useRoster";
import { useQuery } from "@/lib/query";
import { ago } from "@/lib/time";
import { Dialog, Logo, Notice } from "@/ui";
import { t } from "@/lib/i18n";
import { listClosed, type Closed } from "./api";
import { closedTitle, closedWhere } from "./closed";
import { reopen } from "./offer";

export default function ReopenDialog({ onClose }: { onClose: () => void }) {
  const { machines } = useRoster();
  const { data, error, loading } = useQuery(listClosed, []);
  const [busy, setBusy] = useState("");
  const list = useRef<HTMLUListElement>(null);
  const pick = async (c: Closed) => {
    setBusy(c.id);
    await reopen(c.machine, [c.id]);
    setBusy("");
    onClose();
  };
  // ↑ ↓ walk the rows
  const walk = (e: KeyboardEvent<HTMLUListElement>) => {
    if (e.key !== "ArrowDown" && e.key !== "ArrowUp") return;
    const rows = [...(list.current?.querySelectorAll<HTMLButtonElement>(".pins-row") ?? [])];
    const at = rows.indexOf(document.activeElement as HTMLButtonElement);
    const next = rows[Math.max(0, Math.min(rows.length - 1, at + (e.key === "ArrowDown" ? 1 : -1)))];
    if (next) {
      e.preventDefault();
      next.focus();
    }
  };
  const closed = data ?? [];
  return (
    <Dialog title={t("Reopen a closed agent")} icon="refresh-cw" size="md" onClose={onClose}>
      {error && <Notice tone="error">{error}</Notice>}
      {!loading && !error && closed.length === 0 && <Notice tone="hint">{t("No closed agent to bring back. An agent whose tab or workspace closes is listed here, on its own session.")}</Notice>}
      {closed.length > 0 && (
        <ul className="pins-list" aria-label={t("Closed agents")} ref={list} onKeyDown={walk}>
          {closed.map((c, i) => (
            <li key={`${c.machine}|${c.id}`} className="pins-item">
              <button type="button" className="pins-row" autoFocus={i === 0} disabled={!!busy} onClick={() => void pick(c)} title={`${c.dir}\n${t("Start it again on its own session")}`}>
                <Logo agent={c.agent} />
                <span className="pins-line">{closedTitle(c)}</span>
                <span className="pins-when">
                  {busy === c.id ? t("starting…") : [closedWhere(c, machineLabel(machines, c.machine)), ago(c.at)].filter(Boolean).join(" · ")}
                </span>
              </button>
            </li>
          ))}
        </ul>
      )}
    </Dialog>
  );
}
