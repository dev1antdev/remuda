// The row at the activity panel's top: a chip per family of alerts with its count, a Select for the machine, and
// Show everything when anything is on.

import { machineOptions } from "@/features/roster/options";
import type { Machine } from "@/features/roster/api";
import { t } from "@/lib/i18n";
import { Icon, Select } from "@/ui";
import { DEFAULT_FILTER, FAMILIES, OTHER, isFiltering, toggleFamily, type Filter } from "./filter";

// onChange takes the next filter or a function of the one stored, so two quick taps never work from a stale reading
export default function FilterBar({ filter, counts, machines, onChange }: { filter: Filter; counts: Record<string, number>; machines: Machine[]; onChange: (next: Filter | ((current: Filter) => Filter)) => void }) {
  // every family has its chip; the other one is named only while it has entries or is on
  const shown = FAMILIES.filter((family) => family.key !== OTHER || (counts[family.key] ?? 0) > 0 || filter.kinds.includes(family.key));
  return (
    <div className="activity-filter">
      <div className="activity-chips" role="group" aria-label={t("Only these kinds")}>
        {shown.map((family) => {
          const on = filter.kinds.includes(family.key);
          return (
            <button key={family.key} type="button" className={`btn sm activity-chip${on ? " on" : ""}`} aria-pressed={on} data-family={family.key} onClick={() => onChange((current) => toggleFamily(current, family.key))}>
              <Icon name={family.icon} size={12} /> {t(family.label)}
              <span className="badge quiet">{counts[family.key] ?? 0}</span>
            </button>
          );
        })}
      </div>
      <div className="activity-filter-tools">
        <Select className="sm" value={filter.machine} options={machineOptions(machines, { all: t("Everywhere"), onlyConnected: false })} onChange={(machine) => onChange((current) => ({ ...current, machine }))} />
        {isFiltering(filter) && (
          <button type="button" className="btn sm ghost" onClick={() => onChange(DEFAULT_FILTER)}>
            {t("Show everything")}
          </button>
        )}
      </div>
    </div>
  );
}
