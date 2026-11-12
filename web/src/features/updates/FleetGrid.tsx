// The Fleet setup's table: a column per machine, a row per fact under its section's head, each value as a person reads
// it; a cell off the norm wears the amber mark and says on a closer look what most machines have.

import type { ReactNode } from "react";
import Tip from "@/ui/Tip";
import { t } from "@/lib/i18n";
import type { FleetCell, FleetRow, FleetSection, FleetTable } from "./api";
import { cellTip, rowExplain, rowLabel, valueWords } from "./fleet";

// markWords says what a marked cell's mark means: the norm it differs from, or that the row has none
function markWords(row: FleetRow, cell: FleetCell): string {
  if (cell.mark === "split") return t("No value most machines share");
  if (cell.mark === "odd") return t("Most machines: {value}", { value: valueWords(row, { value: row.norm ?? "" }) });
  return "";
}

function Cell({ row, cell, redact }: { row: FleetRow; cell?: FleetCell; redact: (text: string) => string }) {
  if (!cell) return <td className="fleet-cell is-na">—</td>;
  const words = valueWords(row, cell);
  const hash = row.kind === "hash" && !!cell.value && cell.value !== "present";
  // a version, a model id, a branch or a hash is one token: never broken at its hyphens, the table scrolls instead
  const token = hash || words === cell.value;
  const value = <span className={`fleet-value${hash ? " fleet-hash" : ""}${token ? " fleet-token" : ""}`}>{redact(words)}</span>;
  const tip = [markWords(row, cell), redact(cellTip(row, cell))].filter(Boolean).join(" · ");
  return (
    <td className={`fleet-cell${cell.mark ? ` is-${cell.mark}` : ""}`}>
      {tip ? (
        <Tip text={tip}>
          <span className="fleet-tip" tabIndex={0}>
            {value}
          </span>
        </Tip>
      ) : (
        value
      )}
    </td>
  );
}

export default function FleetGrid({ table, sections, name, title, redact }: {
  table: FleetTable;
  sections: FleetSection[];
  name: (machine: string) => string; // a machine as the page names it, the neutral name while the screen is shared
  title: (section: FleetSection) => string;
  redact: (text: string) => string;
}) {
  const head = (machine: FleetTable["machines"][number]): ReactNode => {
    const state = machine.away ? t("away") : machine.error ? t("not read") : "";
    const label = (
      <span className="fleet-col">
        <span>{name(machine.id)}</span>
        {state && <span className="fleet-col-state">{state}</span>}
      </span>
    );
    return machine.error ? (
      <Tip text={redact(machine.error)}>
        <span className="fleet-tip" tabIndex={0}>
          {label}
        </span>
      </Tip>
    ) : (
      label
    );
  };
  return (
    <div className="fleet-scroll">
      <table className="fleet-table">
        <thead>
          <tr>
            <th scope="col" className="fleet-corner">
              {t("Fact")}
            </th>
            {table.machines.map((machine) => (
              <th scope="col" key={machine.id} className={machine.away || machine.error ? "is-away" : undefined}>
                {head(machine)}
              </th>
            ))}
          </tr>
        </thead>
        {sections.map((section) => (
          <tbody key={`${section.id}|${section.name ?? ""}`}>
            <tr className="fleet-section">
              <th scope="rowgroup" colSpan={table.machines.length + 1}>
                {title(section)}
              </th>
            </tr>
            {section.rows.map((row) => (
              <tr key={`${row.id}|${row.arg ?? ""}`}>
                <th scope="row">
                  <Tip text={rowExplain(row)}>
                    <span className="fleet-tip fleet-label" tabIndex={0}>
                      {rowLabel(row)}
                    </span>
                  </Tip>
                </th>
                {table.machines.map((machine) => (
                  <Cell key={machine.id} row={row} cell={row.cells[machine.id]} redact={redact} />
                ))}
              </tr>
            ))}
          </tbody>
        ))}
      </table>
    </div>
  );
}
