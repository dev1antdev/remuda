// A tidy's choices: what it closes, compacts and removes, and on which machines. Whatever is picked is shown again,
// action by action, before anything runs.

import type { Machine } from "@/features/roster/api";
import { useSharing } from "@/features/roster/useSharing";
import type { Tidy } from "./api";
import { DEFAULT_COMPACT_AT, DEFAULT_IDLE_HOURS } from "./scenes";
import { t } from "@/lib/i18n";

// numberIn is a typed number kept within its range, the fallback when the field holds none
const numberIn = (text: string, min: number, max: number, fallback: number) => {
  const n = Math.round(Number(text));
  return Number.isFinite(n) && text.trim() !== "" ? Math.min(max, Math.max(min, n)) : fallback;
};

export default function TidyForm({ tidy, machines, onChange }: { tidy: Tidy; machines: Machine[]; onChange: (t: Tidy) => void }) {
  const set = (patch: Partial<Tidy>) => onChange({ ...tidy, ...patch });
  const share = useSharing(); // under the screen-sharing mode the hidden machines by their neutral names
  const picked = new Set(tidy.machines ?? []);
  const toggleMachine = (id: string) => {
    const next = new Set(picked);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    set({ machines: [...next] });
  };
  return (
    <div className="scene-tidy">
      <span className="scene-steps-head">{t("What it does")}</span>
      <label className="scene-check">
        <input type="checkbox" checked={tidy.shells} onChange={(e) => set({ shells: e.target.checked })} />
        <span>{t("Close the shells sitting at their prompt")}</span>
      </label>
      <label className="scene-check">
        <input type="checkbox" checked={tidy.idle} onChange={(e) => set({ idle: e.target.checked, idle_hours: tidy.idle_hours ?? DEFAULT_IDLE_HOURS })} />
        <span>
          {t("Close agents idle for")}{" "}
          <input
            className="sm scene-number"
            type="number"
            min={1}
            max={168}
            value={tidy.idle_hours ?? DEFAULT_IDLE_HOURS}
            disabled={!tidy.idle}
            onChange={(e) => set({ idle_hours: numberIn(e.target.value, 1, 168, DEFAULT_IDLE_HOURS) })}
            aria-label={t("Hours idle")}
          />{" "}
          {t("hours or more")}
          <small>{t("their sessions stay: Reopen a closed agent brings one back")}</small>
        </span>
      </label>
      <label className="scene-check">
        <input type="checkbox" checked={tidy.compact} onChange={(e) => set({ compact: e.target.checked, compact_at: tidy.compact_at ?? DEFAULT_COMPACT_AT })} />
        <span>
          {t("Send /compact to idle Claude Codes past")}{" "}
          <input
            className="sm scene-number"
            type="number"
            min={10}
            max={95}
            value={tidy.compact_at ?? DEFAULT_COMPACT_AT}
            disabled={!tidy.compact}
            onChange={(e) => set({ compact_at: numberIn(e.target.value, 10, 95, DEFAULT_COMPACT_AT) })}
            aria-label={t("Context share")}
          />{" "}
          {t("% of their context")}
          <small>{t("typed only when you press the scene, as a message to several agents is")}</small>
        </span>
      </label>
      <label className="scene-check">
        <input type="checkbox" checked={tidy.worktrees} onChange={(e) => set({ worktrees: e.target.checked })} />
        <span>{t("Remove the clean leftover worktrees")}</span>
      </label>
      <p className="modal-hint">{t("Never an agent that works, asks you something or has a finish you have not looked at.")}</p>
      {machines.length > 1 && (
        <>
          <span className="scene-steps-head">{t("Machines")} <small>{t("none ticked: every one")}</small></span>
          <div className="scene-machines">
            {machines.map((m) => (
              <label key={m.id} className="scene-check">
                <input type="checkbox" checked={picked.has(m.id)} onChange={() => toggleMachine(m.id)} />
                <span>{share.machineLabel(m)}</span>
              </label>
            ))}
          </div>
        </>
      )}
    </div>
  );
}
