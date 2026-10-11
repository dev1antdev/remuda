// A project's own fields on a task: the value as words (a chip on a card
// or a list row, a line of the prompt) and the control that edits it, by
// the field's kind. A date field is a calendar day, kept as UTC midnight
// so it reads the same in every zone.

import { useEffect, useState } from "react";
import type { Field, FieldValue } from "./tasksApi";
import Select from "./Select";
import Icon from "./icons";
import { openUrl } from "./host";

export function valueText(f: Field, v: FieldValue | undefined): string {
  if (v === undefined || v === null || v === "" || v === false) return "";
  if (Array.isArray(v)) return v.join(", ");
  if (v === true) return "yes";
  if (typeof v === "number") return String(v);
  if (f.kind === "date") {
    const [y, m, d] = v.slice(0, 10).split("-").map(Number);
    if (y && m && d) return new Date(y, m - 1, d).toLocaleDateString([], { day: "numeric", month: "short", year: y === new Date().getFullYear() ? undefined : "numeric" });
  }
  return v;
}

// the value beside a title, when the field asks to be on the card
export function FieldChip({ field, value }: { field: Field; value: FieldValue | undefined }) {
  const text = valueText(field, value);
  if (!text) return null;
  if (field.kind === "check") {
    return (
      <span className="tasks-fieldv" title={field.title}>
        <Icon name="check" size={12} /> {field.title}
      </span>
    );
  }
  return (
    <span className="tasks-fieldv" title={`${field.title}: ${text}`}>
      <span className="tasks-fieldv-name">{field.title}:</span> {text}
    </span>
  );
}

export function FieldInput({ field, value, onCommit }: { field: Field; value: FieldValue | undefined; onCommit: (v: FieldValue) => void }) {
  switch (field.kind) {
    case "text":
    case "link":
    case "number":
      return <TextField field={field} value={value} onCommit={onCommit} />;
    case "select":
      return (
        <Select
          value={typeof value === "string" ? value : ""}
          onChange={(v) => onCommit(v || null)}
          options={[{ value: "", label: "—" }, ...field.options.map((o) => ({ value: o, label: o }))]}
        />
      );
    case "multi": {
      const chosen = Array.isArray(value) ? value : [];
      return (
        <span className="multi-pick">
          {field.options.map((o) => {
            const on = chosen.includes(o);
            return (
              <button
                key={o}
                type="button"
                className={`btn sm${on ? " on" : ""}`}
                aria-pressed={on}
                onClick={() => {
                  const next = on ? chosen.filter((x) => x !== o) : [...chosen, o];
                  onCommit(next.length ? next : null);
                }}
              >
                {o}
              </button>
            );
          })}
          {field.options.length === 0 && <span className="placeholder small">No options yet — add some in the project's settings.</span>}
        </span>
      );
    }
    case "date":
      return <DateField value={typeof value === "string" ? value.slice(0, 10) : ""} onCommit={(d) => onCommit(d ? `${d}T00:00:00Z` : null)} />;
    case "check":
      return <input type="checkbox" className="tasks-check" checked={value === true} onChange={(e) => onCommit(e.target.checked)} aria-label={field.title} />;
  }
}

// text, a link or a number: kept here while typed, sent when the person
// is done (blur or Enter), only when it changed
function TextField({ field, value, onCommit }: { field: Field; value: FieldValue | undefined; onCommit: (v: FieldValue) => void }) {
  const shown = value === undefined || value === null ? "" : String(value);
  const [local, setLocal] = useState(shown);
  useEffect(() => setLocal(shown), [shown]);
  const commit = () => {
    const v = local.trim();
    if (v === shown) return;
    if (field.kind === "number") {
      if (v === "") return onCommit(null);
      const n = Number(v);
      if (Number.isNaN(n)) return setLocal(shown);
      return onCommit(n);
    }
    onCommit(v || null);
  };
  const input = (
    <input
      className="field-text"
      type={field.kind === "number" ? "number" : field.kind === "link" ? "url" : "text"}
      inputMode={field.kind === "number" ? "decimal" : undefined}
      value={local}
      placeholder={field.kind === "link" ? "https://…" : "—"}
      spellCheck={field.kind === "text"}
      onChange={(e) => setLocal(e.target.value)}
      onBlur={commit}
      onKeyDown={(e) => {
        if (e.key === "Enter") (e.target as HTMLInputElement).blur();
        if (e.key === "Escape") setLocal(shown);
      }}
    />
  );
  if (field.kind !== "link") return input;
  return (
    <span className="field-link">
      {input}
      {/^https?:\/\//.test(shown) && (
        <button type="button" className="icon-btn sm" title="Open" aria-label={`Open ${shown}`} onClick={() => openUrl(shown)}>
          <Icon name="external-link" size={14} />
        </button>
      )}
    </span>
  );
}

function DateField({ value, onCommit }: { value: string; onCommit: (ymd: string) => void }) {
  const [local, setLocal] = useState(value);
  useEffect(() => setLocal(value), [value]);
  return (
    <>
      <input
        type="date"
        value={local}
        onChange={(e) => setLocal(e.target.value)}
        onBlur={() => {
          if (local !== value) onCommit(local);
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter") (e.target as HTMLInputElement).blur();
          if (e.key === "Escape") setLocal(value);
        }}
      />
      {value && (
        <button type="button" className="linkish" onClick={() => onCommit("")}>
          clear
        </button>
      )}
    </>
  );
}
